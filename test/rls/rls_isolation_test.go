//go:build integration

// Test de aislamiento cross-tenant por HTTP para ACC-E02 T8.
//
// Levanta Postgres efímero (testcontainers), aplica las migraciones reales de iam-service,
// crea los roles account_app/iam_login con password de prueba y conecta el servidor bajo
// esos roles (NOSUPERUSER NOBYPASSRLS). Luego semilla dos tenants A y B con un admin cada
// uno, y firma tokens para el admin de cada uno. Verifica por HTTP el aislamiento
// cross-tenant en AMBAS direcciones (T8 + T8c): el token de A no puede leer ni escribir
// filas de B, y el token de B no puede leer ni escribir filas de A, en las tablas marcadas
// RLS en T1 (users, tenants, refresh_tokens, revoked_tokens).
// La dirección B→A fue la objeción (A) del gate L4 de T8b (2026-08-31): un solo sentido
// no es evidencia.
//
// Criterio de cierre (ACC-E02 T8):
//   "un token del tenant A no puede leer ni escribir filas del tenant B en ninguna tabla
//    marcada RLS en T1 — verificado por HTTP contra el servicio corriendo".
//
// Para correr sólo este paquete:
//   cd platform/iam-service && GOWORK=off go test -tags=integration ./test/rls/... -count=1 -v
package rls_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"iam"

	"iam/src/auth/domain/value_object"

	"iam/src/auth/infrastructure/adapter"
	authconfig "iam/src/auth/infrastructure/config"
	authmw "iam/src/auth/infrastructure/middleware"
	"iam/src/auth/infrastructure/s2s"
	"iam/src/shared/validator"
	planconfig "iam/src/plan/infrastructure/config"
	roleconfig "iam/src/role/infrastructure/config"
	tenantconfig "iam/src/tenant/infrastructure/config"
	userconfig "iam/src/user/infrastructure/config"
	useruc "iam/src/user/application/usecase"
	userrepo "iam/src/user/infrastructure/persistence/repository"

	sharedport "github.com/hornosg/go-shared/domain/port"
	sharedmigrate "github.com/hornosg/go-shared/migrate"
)

const (
	testNamespace      = "mc"
	testJWTSecret      = "test-jwt-secret-must-be-at-least-32-bytes-long"
	testAppPassword     = "test_account_app"
	testLoginPassword   = "test_iam_login"
	testAdminPassword   = "StrongP@ssw0rd"
	// ACC-E02 T8d: keys S2S de prueba con las políticas reales del ServicePolicy
	// (sales → system:admin; whatsapp-agent → tenant:provision a secas).
	testSalesKey    = "test-sales-s2s-key-0123456789abcdef"
	testWhatsappKey = "test-whatsapp-s2s-key-0123456789abcdef"
)

// testRLSServer levanta PostgreSQL + migraciones + router IAM conectado como account_app/iam_login.
type testRLSServer struct {
	Server   *httptest.Server
	AppDB    *sql.DB
	LoginDB  *sql.DB
	SuperDB  *sql.DB
	TenantA  uuid.UUID
	TenantB  uuid.UUID
	UserA    uuid.UUID
	UserB    uuid.UUID
	TokenA   string
	TokenB   string
	RoleID   uuid.UUID
}

// noopMetricsRecorder implementa sharedport.MetricsRecorder para tests.
type noopMetricsRecorder struct{}

func (noopMetricsRecorder) Record(_ sharedport.MetricEvent) {}

func newTestRLSServer(t *testing.T) *testRLSServer {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("iam_db"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = pgContainer.Terminate(ctx)
	})

	superConnStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	superDB, err := sql.Open("postgres", superConnStr)
	require.NoError(t, err)
	require.NoError(t, superDB.PingContext(ctx))

	// Aplicar migraciones como superuser (account_app no puede crear roles).
	require.NoError(t, sharedmigrate.RunMigrations(superDB, iam.MigrationsFS, "iam_db"))

	// Fijar passwords de los roles de aplicación para poder conectar desde el test.
	_, err = superDB.Exec("ALTER ROLE account_app WITH PASSWORD '" + testAppPassword + "'")
	require.NoError(t, err)
	_, err = superDB.Exec("ALTER ROLE iam_login WITH PASSWORD '" + testLoginPassword + "'")
	require.NoError(t, err)

	appConnStr, err := withCredentials(superConnStr, "account_app", testAppPassword)
	require.NoError(t, err)
	appDB, err := sql.Open("postgres", appConnStr)
	require.NoError(t, err)
	require.NoError(t, appDB.PingContext(ctx))

	loginConnStr, err := withCredentials(superConnStr, "iam_login", testLoginPassword)
	require.NoError(t, err)
	loginDB, err := sql.Open("postgres", loginConnStr)
	require.NoError(t, err)
	require.NoError(t, loginDB.PingContext(ctx))

	// Verificar que ambos pools corren bajo roles NOBYPASSRLS (T6).
	require.NoError(t, assertNoRLSBypass(appDB), "account_app debe ser NOBYPASSRLS")
	require.NoError(t, assertNoRLSBypass(loginDB), "iam_login debe ser NOBYPASSRLS")

	// Semilla: dos tenants + un admin de tenant_admin en cada uno.
	tenantA := uuid.New()
	tenantB := uuid.New()
	userA := uuid.New()
	userB := uuid.New()
	tenantAdminRoleID := queryTenantAdminRoleID(t, superDB)
	seedTenantAndUser(t, superDB, tenantA, userA, "admin-a@example.com", "tenant-a", tenantAdminRoleID)
	seedTenantAndUser(t, superDB, tenantB, userB, "admin-b@example.com", "tenant-b", tenantAdminRoleID)

	// Tokens firmados directamente para A y B. Evitamos depender del path de login
	// pre-auth (iam_login), que hoy lee columnas no autorizadas de users — bug heredado
	// de T2/T5, fuera del scope de T8.
	tokenA := makeAccessToken(t, userA, tenantA, tenantAdminRoleID, "admin-a@example.com")
	tokenB := makeAccessToken(t, userB, tenantB, tenantAdminRoleID, "admin-b@example.com")

	// Construir router exactamente como main.go pero sobre los pools de test.
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery())
	validator.RegisterCustomValidators()

	apiV1 := router.Group("/api/v1")
	metricsRecorder := noopMetricsRecorder{}

	s2sRegistry := s2s.LoadFromEnvForTests(map[string]string{
		"sales":          testSalesKey,
		"whatsapp-agent": testWhatsappKey,
	})

	// ACC-E02 T8g (criterio (a)): el gate de revocación se registra sobre apiV1
	// ANTES de crear los grupos de gestión — Gin congela la cadena de handlers al
	// crear el grupo, y un Use() posterior sobre el padre no los alcanza.
	authRepoApp := authconfig.SetupTokenRevocationGate(apiV1, appDB, loginDB, testJWTSecret)

	authFactory := authmw.NewScopeMiddlewareFactory(testJWTSecret, testNamespace, s2sRegistry)
	adminGroup := apiV1.Group("", authFactory.RequireScope(s2s.ScopeSystemAdmin, "system_admin"))
	tenantScopedGroup := apiV1.Group("", authFactory.RequireScopes([]s2s.Scope{s2s.ScopeSystemAdmin, s2s.ScopeTenantAdmin}, "tenant_admin", "system_admin"))

	userFinderService := userconfig.SetupUserModule(tenantScopedGroup, appDB)
	loginUserRepo := userrepo.NewPostgresUserRepository(loginDB)
	loginUserFinder := useruc.NewUserFinderUseCase(loginUserRepo)

	tenantconfig.SetupTenantScopedModule(tenantScopedGroup, appDB, metricsRecorder)
	tenantFeaturesUC := tenantconfig.SetupTenantModule(adminGroup, appDB, metricsRecorder)
	tenantService := adapter.NewTenantFeaturesAdapter(tenantFeaturesUC)

	authCfg := authconfig.AuthModuleConfig{
		JWTSecret:          testJWTSecret,
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		Namespace:          testNamespace,
	}
	authconfig.SetupAuthModule(apiV1, appDB, loginDB, authRepoApp, userFinderService, loginUserFinder, tenantService, authCfg)

	planconfig.SetupPlanModule(adminGroup, appDB)
	roleconfig.SetupRoleModule(tenantScopedGroup, adminGroup, appDB)

	provisionGroup := apiV1.Group("", authFactory.RequireScopes([]s2s.Scope{s2s.ScopeTenantProvision, s2s.ScopeSystemAdmin}, "system_admin"))
	tenantconfig.SetupTenantProvisionModule(provisionGroup, appDB, metricsRecorder)

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	ts := &testRLSServer{
		Server:  srv,
		AppDB:   appDB,
		LoginDB: loginDB,
		SuperDB: superDB,
		TenantA: tenantA,
		TenantB: tenantB,
		UserA:   userA,
		UserB:   userB,
		TokenA:  tokenA,
		TokenB:  tokenB,
		RoleID:  tenantAdminRoleID,
	}
	return ts
}

func withCredentials(connStr, user, password string) (string, error) {
	u, err := url.Parse(connStr)
	if err != nil {
		return "", err
	}
	u.User = url.UserPassword(user, password)
	return u.String(), nil
}

func assertNoRLSBypass(db *sql.DB) error {
	var privileged bool
	if err := db.QueryRow(
		`SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname = current_user`,
	).Scan(&privileged); err != nil {
		return fmt.Errorf("no se pudo verificar los privilegios del rol de DB (current_user): %w", err)
	}
	if privileged {
		return fmt.Errorf("negativa a arrancar: el rol de DB actual es SUPERUSER o BYPASSRLS y eludiría la row-level security")
	}
	return nil
}

func queryTenantAdminRoleID(t *testing.T, db *sql.DB) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := db.QueryRow("SELECT id FROM roles WHERE slug = 'tenant_admin' AND is_active = true").Scan(&id)
	require.NoError(t, err)
	return id
}

func seedTenantAndUser(t *testing.T, db *sql.DB, tenantID, userID uuid.UUID, email, slug string, roleID uuid.UUID) {
	t.Helper()
	// Hash de "123456", mismo que el seed de migración 006. No se usa para login acá.
	const passwordHash = "$2a$10$yMecfP7H7mT0m9VHNnMFIezB06ihuIqUVpD12sa34UHhSfCRIQdje"

	_, err := db.Exec(`
		INSERT INTO tenants (id, name, slug, description, type, status, owner_id, max_users, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'BUSINESS', 'ACTIVE', $5, 100, NOW(), NOW())`,
		tenantID, slug, slug, "tenant "+slug, userID)
	require.NoError(t, err)

	_, err = db.Exec(`
		INSERT INTO users (id, email, password_hash, tenant_id, role_id, status, provider, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'ACTIVE', 'LOCAL', NOW(), NOW())`,
		userID, email, passwordHash, tenantID, roleID)
	require.NoError(t, err)
}

func makeAccessToken(t *testing.T, userID, tenantID, roleID uuid.UUID, email string) string {
	t.Helper()
	claims := value_object.NewTokenClaims(
		userID,
		tenantID,
		roleID,
		email,
		testNamespace,
		value_object.DefaultTenantFeatures(),
		time.Now().Add(15*time.Minute),
	)
	claims.Roles = []string{"tenant_admin"}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, adapter.JWTClaims{TokenClaims: *claims})
	tokenStr, err := token.SignedString([]byte(testJWTSecret))
	require.NoError(t, err)
	return tokenStr
}

func postJSON(t *testing.T, url, token string, body interface{}) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func get(t *testing.T, url, token string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func putJSON(t *testing.T, url, token string, body interface{}, headers map[string]string) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(b))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func del(t *testing.T, url, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func TestRLS_CrossTenantIsolation(t *testing.T) {
	ts := newTestRLSServer(t)
	base := ts.Server.URL + "/api/v1"

	t.Run("token A es válido y puede autenticar", func(t *testing.T) {
		require.NotEmpty(t, ts.TokenA)
		url := fmt.Sprintf("%s/tenants/%s", base, ts.TenantA.String())
		resp := get(t, url, ts.TokenA, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("A puede listar su propio user", func(t *testing.T) {
		url := base + "/users"
		resp := get(t, url, ts.TokenA, map[string]string{"X-Tenant-ID": ts.TenantA.String()})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var listResp map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
		items, ok := listResp["items"].([]interface{})
		require.True(t, ok, "expected items array")
		emails := []string{}
		for _, it := range items {
			m, ok := it.(map[string]interface{})
			require.True(t, ok)
			emails = append(emails, m["email"].(string))
		}
		assert.Contains(t, emails, "admin-a@example.com")
		assert.NotContains(t, emails, "admin-b@example.com")
	})

	t.Run("A NO puede leer el user de B", func(t *testing.T) {
		url := fmt.Sprintf("%s/users/%s", base, ts.UserB.String())
		resp := get(t, url, ts.TokenA, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("A NO puede actualizar el user de B", func(t *testing.T) {
		url := fmt.Sprintf("%s/users/%s", base, ts.UserB.String())
		body := map[string]interface{}{"first_name": "Hackeado", "last_name": "X"}
		resp := putJSON(t, url, ts.TokenA, body, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("A puede leer su propio tenant", func(t *testing.T) {
		url := fmt.Sprintf("%s/tenants/%s", base, ts.TenantA.String())
		resp := get(t, url, ts.TokenA, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("A NO puede leer el tenant de B", func(t *testing.T) {
		url := fmt.Sprintf("%s/tenants/%s", base, ts.TenantB.String())
		resp := get(t, url, ts.TokenA, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	// — Dirección B→A (T8c): recíproco simétrico de los subtests de arriba.
	// tokenB nunca fue emisor antes de T8c (objeción (A) del gate L4 de T8b).

	t.Run("token B es válido y puede autenticar", func(t *testing.T) {
		require.NotEmpty(t, ts.TokenB)
		url := fmt.Sprintf("%s/users/%s", base, ts.UserB.String())
		resp := get(t, url, ts.TokenB, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("B puede listar su propio user", func(t *testing.T) {
		url := base + "/users"
		resp := get(t, url, ts.TokenB, map[string]string{"X-Tenant-ID": ts.TenantB.String()})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var listResp map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
		items, ok := listResp["items"].([]interface{})
		require.True(t, ok, "expected items array")
		emails := []string{}
		for _, it := range items {
			m, ok := it.(map[string]interface{})
			require.True(t, ok)
			emails = append(emails, m["email"].(string))
		}
		assert.Contains(t, emails, "admin-b@example.com")
		assert.NotContains(t, emails, "admin-a@example.com")
	})

	t.Run("B NO puede leer el user de A", func(t *testing.T) {
		url := fmt.Sprintf("%s/users/%s", base, ts.UserA.String())
		resp := get(t, url, ts.TokenB, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("B NO puede actualizar el user de A", func(t *testing.T) {
		url := fmt.Sprintf("%s/users/%s", base, ts.UserA.String())
		body := map[string]interface{}{"first_name": "Hackeado", "last_name": "X"}
		resp := putJSON(t, url, ts.TokenB, body, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("B puede leer su propio tenant", func(t *testing.T) {
		url := fmt.Sprintf("%s/tenants/%s", base, ts.TenantB.String())
		resp := get(t, url, ts.TokenB, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("B NO puede leer el tenant de A", func(t *testing.T) {
		url := fmt.Sprintf("%s/tenants/%s", base, ts.TenantA.String())
		resp := get(t, url, ts.TokenB, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	// — Escrituras cross-tenant (criterio T8: "leer NI ESCRIBIR en NINGUNA
	// tabla RLS"). Arriba está la lectura (GET) y el UPDATE de users; faltaban
	// el UPDATE de tenants, el DELETE de users y el DELETE de tenants, en las
	// dos direcciones. Cada negativo se ancla con aserción de no-mutación vía
	// SuperDB y los positivos propios distinguen "RLS aísla" de "ruta rota
	// que da 404 universal" — el mismo control que T8c exigió para la lectura.

	t.Run("A puede actualizar su propio tenant", func(t *testing.T) {
		// Control positivo de PUT /tenants/:id: el 404 cross-tenant de abajo
		// sólo prueba aislamiento si esta misma ruta responde 200 para A.
		// ⚠ El use case sólo aplica UpdateDetails cuando llegan name Y
		// description juntos (update_tenant.go: `req.Name != nil && req.Description
		// != nil`) — mandar sólo name es un 200 no-op, bug pre-existente fuera
		// del alcance de T8. Acá se mandan ambos para que el positivo mute.
		url := fmt.Sprintf("%s/tenants/%s", base, ts.TenantA.String())
		body := map[string]interface{}{
			"name":        "Tenant A Renombrado",
			"description": "Descripción renovada del tenant A",
		}
		resp := putJSON(t, url, ts.TokenA, body, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var nombre string
		require.NoError(t, ts.SuperDB.QueryRow(
			`SELECT name FROM tenants WHERE id = $1`, ts.TenantA).Scan(&nombre))
		assert.Equal(t, "Tenant A Renombrado", nombre,
			"el control positivo debe mutar de verdad: un 200 sin persistencia no distingue nada")
	})

	t.Run("A NO puede actualizar el tenant de B", func(t *testing.T) {
		url := fmt.Sprintf("%s/tenants/%s", base, ts.TenantB.String())
		body := map[string]interface{}{"name": "Hackeado"}
		resp := putJSON(t, url, ts.TokenA, body, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		var nombre string
		require.NoError(t, ts.SuperDB.QueryRow(
			`SELECT name FROM tenants WHERE id = $1`, ts.TenantB).Scan(&nombre))
		assert.Equal(t, "tenant-b", nombre,
			"el PUT cross-tenant no debe mutar la fila de B")
	})

	t.Run("A NO puede eliminar el user de B", func(t *testing.T) {
		url := fmt.Sprintf("%s/users/%s", base, ts.UserB.String())
		resp := del(t, url, ts.TokenA)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		var status string
		require.NoError(t, ts.SuperDB.QueryRow(
			`SELECT status FROM users WHERE id = $1`, ts.UserB).Scan(&status))
		assert.Equal(t, "ACTIVE", status,
			"el DELETE cross-tenant no debe mutar la fila de B")
	})

	t.Run("A NO puede eliminar el tenant de B", func(t *testing.T) {
		url := fmt.Sprintf("%s/tenants/%s", base, ts.TenantB.String())
		resp := del(t, url, ts.TokenA)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		var status string
		require.NoError(t, ts.SuperDB.QueryRow(
			`SELECT status FROM tenants WHERE id = $1`, ts.TenantB).Scan(&status))
		assert.Equal(t, "ACTIVE", status,
			"el DELETE cross-tenant no debe mutar la fila de B")
	})

	// Dirección B→A de las escrituras (T8c: un solo sentido no es evidencia).

	t.Run("B NO puede actualizar el tenant de A", func(t *testing.T) {
		url := fmt.Sprintf("%s/tenants/%s", base, ts.TenantA.String())
		body := map[string]interface{}{"name": "Hackeado B"}
		resp := putJSON(t, url, ts.TokenB, body, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		var nombre string
		require.NoError(t, ts.SuperDB.QueryRow(
			`SELECT name FROM tenants WHERE id = $1`, ts.TenantA).Scan(&nombre))
		assert.Equal(t, "Tenant A Renombrado", nombre,
			"el PUT cross-tenant no debe mutar la fila de A")
	})

	t.Run("B NO puede eliminar el user de A", func(t *testing.T) {
		url := fmt.Sprintf("%s/users/%s", base, ts.UserA.String())
		resp := del(t, url, ts.TokenB)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		var status string
		require.NoError(t, ts.SuperDB.QueryRow(
			`SELECT status FROM users WHERE id = $1`, ts.UserA).Scan(&status))
		assert.Equal(t, "ACTIVE", status,
			"el DELETE cross-tenant no debe mutar la fila de A")
	})

	t.Run("B NO puede eliminar el tenant de A", func(t *testing.T) {
		url := fmt.Sprintf("%s/tenants/%s", base, ts.TenantA.String())
		resp := del(t, url, ts.TokenB)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		var status string
		require.NoError(t, ts.SuperDB.QueryRow(
			`SELECT status FROM tenants WHERE id = $1`, ts.TenantA).Scan(&status))
		assert.Equal(t, "ACTIVE", status,
			"el DELETE cross-tenant no debe mutar la fila de A")
	})

	t.Run("A puede eliminar un user de su propio tenant", func(t *testing.T) {
		// Control positivo de DELETE /users/:id: mismo motivo que el positivo
		// de PUT — sin él, el 404 cross-tenant podría ser una ruta ausente.
		userExtra := uuid.New()
		_, err := ts.SuperDB.Exec(`
			INSERT INTO users (id, email, password_hash, tenant_id, role_id, status, provider, created_at, updated_at)
			VALUES ($1, 'extra-a@example.com', $2, $3, $4, 'ACTIVE', 'LOCAL', NOW(), NOW())`,
			userExtra, "$2a$10$yMecfP7H7mT0m9VHNnMFIezB06ihuIqUVpD12sa34UHhSfCRIQdje", ts.TenantA, ts.RoleID)
		require.NoError(t, err)

		url := fmt.Sprintf("%s/users/%s", base, userExtra.String())
		resp := del(t, url, ts.TokenA)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("A puede eliminar su propio tenant", func(t *testing.T) {
		// Control positivo de DELETE /tenants/:id (soft delete). Va ÚLTIMO:
		// soft-deletea tenantA, que todos los subtests anteriores usan como
		// emisor y objetivo.
		url := fmt.Sprintf("%s/tenants/%s", base, ts.TenantA.String())
		resp := del(t, url, ts.TokenA)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})
}

// — Tokens de sesión (refresh_tokens / revoked_tokens): objeción (B) del gate
// L4 de T8b (2026-08-31), encuadrada en T8. Las policies de estas dos tablas
// son una subselect `user_id IN (...)` — distintas de las de users/tenants —
// y merecen evidencia HTTP propia. Los controles positivos (A refresca el suyo,
// B refresca el suyo) distinguen aislamiento correcto de un fail-closed global,
// que es exactamente el modo de fallo que el checkpoint de T8 (2026-08-29)
// encontró en users/tenants antes de T8b.
func seedRefreshToken(t *testing.T, db *sql.DB, tenantID, userID uuid.UUID, token string) {
	t.Helper()
	// T8e: la 021 denormaliza tenant_id como NOT NULL — el seed lo trae, igual
	// que hace CreateRefreshToken en la app (INSERT con tenant_id, WITH CHECK).
	_, err := db.Exec(`
		INSERT INTO refresh_tokens (id, user_id, tenant_id, token, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())`,
		uuid.New(), userID, tenantID, token, time.Now().Add(7*24*time.Hour))
	require.NoError(t, err)
}

func seedRevokedJTI(t *testing.T, db *sql.DB, jti, userID uuid.UUID) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO revoked_tokens (jti, user_id, expires_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (jti) DO NOTHING`,
		jti, userID, time.Now().Add(24*time.Hour))
	require.NoError(t, err)
}

// jtiDe extrae el JTI de un access token firmado con el secreto de test.
func jtiDe(t *testing.T, token string) uuid.UUID {
	t.Helper()
	claims := &adapter.JWTClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (interface{}, error) {
		return []byte(testJWTSecret), nil
	})
	require.NoError(t, err)
	require.True(t, parsed.Valid)
	require.NotEqual(t, uuid.Nil, claims.TokenClaims.JTI)
	return claims.TokenClaims.JTI
}

func TestRLS_TokensDeSesion(t *testing.T) {
	ts := newTestRLSServer(t)
	base := ts.Server.URL + "/api/v1"

	// Un refresh token por subtest: el refresh consume el token (DELETE), así
	// que reutilizar uno acoplaría subtests entre sí.
	seedRefreshToken(t, ts.SuperDB, ts.TenantA, ts.UserA, "rt-a-propio")
	seedRefreshToken(t, ts.SuperDB, ts.TenantA, ts.UserA, "rt-a-para-b")
	seedRefreshToken(t, ts.SuperDB, ts.TenantB, ts.UserB, "rt-b-propio")
	seedRefreshToken(t, ts.SuperDB, ts.TenantB, ts.UserB, "rt-b-para-a")

	t.Run("B puede refrescar su propio refresh token", func(t *testing.T) {
		resp := postJSON(t, base+"/auth/refresh", "", map[string]string{"refresh_token": "rt-b-propio"})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("A puede refrescar su propio refresh token", func(t *testing.T) {
		resp := postJSON(t, base+"/auth/refresh", "", map[string]string{"refresh_token": "rt-a-propio"})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("A NO puede refrescar el refresh token de B", func(t *testing.T) {
		resp := postJSON(t, base+"/auth/refresh", ts.TokenA, map[string]string{"refresh_token": "rt-b-para-a"})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("B NO puede refrescar el refresh token de A", func(t *testing.T) {
		resp := postJSON(t, base+"/auth/refresh", ts.TokenB, map[string]string{"refresh_token": "rt-a-para-b"})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("logout de B responde 204 habiendo revocado de verdad", func(t *testing.T) {
		// El 204 solo no dice nada: antes el INSERT de RevokeToken violaba el
		// WITH CHECK (sin app.tenant_id) y el error se descartaba en logout.go
		// con `_ =` — el logout respondía 204 sin haber revocado nada (defecto
		// (2) del checkpoint de T8). Se exige el JTI presente en revoked_tokens
		// Y que el propio gate rechace el token recién revocado con 401.
		resp := postJSON(t, base+"/auth/logout", ts.TokenB, map[string]string{})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)

		jti := jtiDe(t, ts.TokenB)
		var revocado bool
		require.NoError(t, ts.SuperDB.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM revoked_tokens WHERE jti = $1)`, jti).Scan(&revocado),
			"el logout debe dejar el JTI en revoked_tokens")
		assert.True(t, revocado, "el JTI del token de B debe estar revocado tras el logout")

		resp2 := postJSON(t, base+"/auth/revoke-all", ts.TokenB, map[string]string{})
		defer resp2.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp2.StatusCode,
			"un token cuyo JTI quedó revocado debe ser rechazado con 401, no llegar al handler")
	})

	t.Run("un JTI revocado de B es rechazado por el gate de revocación", func(t *testing.T) {
		// Sembrado directo (superuser) del JTI de tokenB en revoked_tokens: el
		// gate TokenRevocationCheck debe verlo y abortar con 401 ANTES de
		// llegar al handler. Si llega al handler (500/200), la lectura de
		// revocación está fallando abierta.
		seedRevokedJTI(t, ts.SuperDB, jtiDe(t, ts.TokenB), ts.UserB)
		resp := postJSON(t, base+"/auth/revoke-all", ts.TokenB, map[string]string{})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("las revocaciones de B no alcanzan al access token de A (dirección B→A)", func(t *testing.T) {
		// T8: revoked_tokens cierra sus dos direcciones. B ya dejó dos marcas
		// en esta corrida (el logout scope='jti' y el JTI sembrado); ninguna
		// puede tocar la sesión de A: el INSERT de revocación corre bajo RLS
		// (policy por user_id IN users del tenant, migraciones 019/021) y el
		// predicado del gate filtra por user_id. La dirección opuesta (el
		// corte de A no alcanza a B) vive en TestRLS_RevokeAllCortePorUsuario.
		resp := get(t, base+"/users", ts.TokenA, map[string]string{"X-Tenant-ID": ts.TenantA.String()})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode,
			"las marcas de revocación de B no deben alcanzar la sesión de A")
	})
}

// — Rotación single-use concurrente (T8f): hallazgo 1 del gate L4 de T8e-2.
// La rotación es check-then-act en transacciones separadas y el DELETE de
// DeleteRefreshToken descartaba RowsAffected: dos refresh concurrentes con el
// MISMO token pasaban ambos el escape de presentación, uno borraba 1 y el otro
// 0 sin error, y AMBOS emitían credenciales nuevas. Bajo READ COMMITTED el
// segundo DELETE se bloquea en el row lock del primero, re-evalúa el WHERE,
// ve 0 filas y aborta: exactamente un request gana. El test corre contra
// Postgres real — los row locks no existen en un mock, y un test secuencial
// nunca ve una lost update (tx-consistency-go).
func TestRLS_RotacionSingleUseConcurrente(t *testing.T) {
	ts := newTestRLSServer(t)
	base := ts.Server.URL + "/api/v1"

	const rt = "rt-single-use-concurrente"
	seedRefreshToken(t, ts.SuperDB, ts.TenantA, ts.UserA, rt)

	// Los dos requests corren en goroutines: postJSON exige (require) sobre el
	// *testing.T del padre, que no es válido fuera del goroutine de test.
	doRefresh := func() int {
		b, err := json.Marshal(map[string]string{"refresh_token": rt})
		if err != nil {
			return -1
		}
		req, err := http.NewRequest(http.MethodPost, base+"/auth/refresh", bytes.NewReader(b))
		if err != nil {
			return -1
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return -1
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	const intentos = 2
	codigos := make(chan int, intentos)
	var wg sync.WaitGroup
	for i := 0; i < intentos; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codigos <- doRefresh()
		}()
	}
	wg.Wait()
	close(codigos)

	exitosos, rechazados, otros := 0, 0, 0
	for codigo := range codigos {
		switch codigo {
		case http.StatusOK:
			exitosos++
		case http.StatusUnauthorized:
			rechazados++
		default:
			otros++
		}
	}

	assert.Equal(t, 1, exitosos,
		"exactamente UNA de las dos rotaciones con el mismo token puede tener éxito: es la propiedad single-use")
	assert.Equal(t, 1, rechazados,
		"el request que perdió la carrera debe recibir 401 (credencial inválida), no 500")
	assert.Equal(t, 0, otros,
		"ningún otro código de estado es admisible: ni 500 (carrera tratada como infraestructura) ni 2xx doble")

	// Evidencia en la base de la consecuencia del hallazgo: si ambos hubieran
	// tenido éxito, quedarían DOS credenciales vivas nacidas de una.
	var restantes int
	require.NoError(t, ts.SuperDB.QueryRow(
		`SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1`, ts.UserA).Scan(&restantes))
	assert.Equal(t, 1, restantes,
		"sólo la rotación exitosa emite un reemplazo para el usuario")
}

// — Gate de revocación sobre las rutas de gestión (ACC-E02 T8g, criterio (b)).
// Hallazgo 3 del gate L4 de T8e-2: adminGroup/tenantScopedGroup se creaban
// ANTES del Use() del gate sobre apiV1, y Gin congela la cadena de handlers al
// crear el grupo → un JTI revocado seguía autorizando /users, /tenants/:id,
// /roles y /plans hasta la expiración por tiempo (≤15 min). Logout y revoke-all
// no cortaban el acceso a esas rutas. La evidencia exige UN endpoint de cada
// grupo Y controles positivos previos: sin ellos, un 401 universal (fail-closed
// global) sería indistinguible de aislamiento correcto — el mismo modo de fallo
// que el checkpoint de T8 (2026-08-29) detectó en users/tenants.
func TestRLS_GateRevocacionCubreGestion(t *testing.T) {
	ts := newTestRLSServer(t)
	base := ts.Server.URL + "/api/v1"

	// Controles positivos: sin revocación, los cuatro endpoints autorizan.
	// /users y /tenants/:id son tenantScopedGroup; /roles (lectura) es
	// tenantScopedGroup; /plans es adminGroup (system:admin → key S2S de sales,
	// que tiene scope system:admin).
	t.Run("sin revocación, los endpoints de gestión autorizan", func(t *testing.T) {
		resp := get(t, base+"/users", ts.TokenA, map[string]string{"X-Tenant-ID": ts.TenantA.String()})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "/users debe autorizar un token vivo")

		resp2 := get(t, fmt.Sprintf("%s/tenants/%s", base, ts.TenantA.String()), ts.TokenA, nil)
		defer resp2.Body.Close()
		assert.Equal(t, http.StatusOK, resp2.StatusCode, "/tenants/:id debe autorizar un token vivo")

		resp3 := get(t, base+"/roles", ts.TokenA, nil)
		defer resp3.Body.Close()
		assert.Equal(t, http.StatusOK, resp3.StatusCode, "/roles debe autorizar un token vivo")

		resp4 := get(t, base+"/plans", "", map[string]string{"X-API-Key": testSalesKey})
		defer resp4.Body.Close()
		assert.Equal(t, http.StatusOK, resp4.StatusCode, "/plans debe autorizar una key S2S con system:admin")
	})

	// Se revoca el JTI del token de A (seed directo como superuser, mismo
	// mecanismo que TestRLS_TokensDeSesion).
	seedRevokedJTI(t, ts.SuperDB, jtiDe(t, ts.TokenA), ts.UserA)

	t.Run("JTI revocado → 401 en los endpoints de gestión", func(t *testing.T) {
		// El gate aborta ANTES de la autorización de scope: la respuesta es 401
		// (token revocado), no 403 (scope insuficiente) ni 500.
		resp := get(t, base+"/users", ts.TokenA, map[string]string{"X-Tenant-ID": ts.TenantA.String()})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "logout/revoke-all deben cortar también /users")

		resp2 := get(t, fmt.Sprintf("%s/tenants/%s", base, ts.TenantA.String()), ts.TokenA, nil)
		defer resp2.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp2.StatusCode, "logout/revoke-all deben cortar también /tenants/:id")

		resp3 := get(t, base+"/roles", ts.TokenA, nil)
		defer resp3.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp3.StatusCode, "logout/revoke-all deben cortar también /roles")

		resp4 := get(t, base+"/plans", ts.TokenA, nil)
		defer resp4.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp4.StatusCode, "logout/revoke-all deben cortar también /plans")
	})

	t.Run("S2S sin JWT sigue pasando el gate (no hay JTI que revocar)", func(t *testing.T) {
		// El gate es pasivo sin Authorization header: una key S2S con scope no
		// debe verse afectada por la revocación de JTIs ajenos.
		resp := get(t, base+"/plans", "", map[string]string{"X-API-Key": testSalesKey})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func makeLegacyAccessToken(t *testing.T, userID, roleID uuid.UUID, email string) string {
	t.Helper()
	// Sin NewTokenClaims: ese constructor exige tenant. Este es el shape de un
	// token emitido antes de que el claim tenant_id existiera.
	claims := value_object.TokenClaims{
		JTI:       uuid.New(),
		Issuer:    "iam-service",
		Namespace: testNamespace,
		UserID:    userID,
		Email:     email,
		TenantID:  uuid.Nil,
		RoleID:    roleID,
		Roles:     []string{"tenant_admin"},
		Features:  value_object.DefaultTenantFeatures(),
		ExpiresAt: time.Now().Add(15 * time.Minute).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, adapter.JWTClaims{TokenClaims: claims})
	tokenStr, err := token.SignedString([]byte(testJWTSecret))
	require.NoError(t, err)
	return tokenStr
}

func TestRLS_LogoutTokenLegacySinTenant(t *testing.T) {
	ts := newTestRLSServer(t)
	base := ts.Server.URL + "/api/v1"

	seedRefreshToken(t, ts.SuperDB, ts.TenantA, ts.UserA, "rt-legacy-logout")
	legacyToken := makeLegacyAccessToken(t, ts.UserA, ts.RoleID, "admin-a@example.com")

	t.Run("token legacy autoriza gestión con tenant derivado (no 500 en bloque)", func(t *testing.T) {
		// El "ojo al implementar" del gate de T8e-1 (hallazgo 6): extender el
		// gate a más rutas ensancha la superficie de 500 para tokens legacy.
		// La derivación la cierra: el token legacy ve SU tenant (aislamiento
		// intacto — no ve los users de B), no un 500.
		url := fmt.Sprintf("%s/users", base)
		resp := get(t, url, legacyToken, map[string]string{"X-Tenant-ID": ts.TenantA.String()})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("logout del token legacy revoca de verdad (204, no 500)", func(t *testing.T) {
		resp := postJSON(t, base+"/auth/logout", legacyToken, map[string]string{})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNoContent, resp.StatusCode,
			"un token sin claim de tenant debe poder cerrar su sesión: el tenant se deriva del user_id verificado")

		var jti, uid uuid.UUID
		parsedClaims := &adapter.JWTClaims{}
		_, err := jwt.ParseWithClaims(legacyToken, parsedClaims, func(*jwt.Token) (interface{}, error) {
			return []byte(testJWTSecret), nil
		})
		require.NoError(t, err)
		require.NoError(t, ts.SuperDB.QueryRow(
			`SELECT jti, user_id FROM revoked_tokens WHERE user_id = $1 LIMIT 1`, ts.UserA).
			Scan(&jti, &uid), "el logout del token legacy debe dejar el JTI en revoked_tokens")
		assert.Equal(t, parsedClaims.TokenClaims.JTI, jti,
			"el JTI revocado debe ser el del token legacy que cerró sesión")
		assert.Equal(t, ts.UserA, uid)

		// DeleteAllUserRefreshTokens también corrió: la sesión quedó cerrada de
		// verdad, no sólo revocado el access token.
		var restantes int
		require.NoError(t, ts.SuperDB.QueryRow(
			`SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1`, ts.UserA).Scan(&restantes))
		assert.Equal(t, 0, restantes, "el logout debe borrar también los refresh tokens del usuario")
	})

	t.Run("el token legacy ya revocado es rechazado por el gate", func(t *testing.T) {
		resp := postJSON(t, base+"/auth/revoke-all", legacyToken, map[string]string{})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"tras el logout, el gate debe rechazar el token legacy con 401 (JTI en revoked_tokens)")
	})
}

// — Revoke-all por corte de usuario (T8i): hallazgo del gate L4 de T8g.
// Antes, RevokeAllUserTokens insertaba en revoked_tokens un JTI ALEATORIO que
// el gate jamás consulta (IsTokenRevoked busca el JTI del token PRESENTADO):
// POST /auth/revoke-all respondía OK habiendo insertado una fila decorativa y
// TODOS los access tokens del usuario seguían válidos hasta expirar. Quien
// sospecha que le robaron la sesión y pide cerrar todas, no cerraba ninguna.
//
// La semántica materializada (migración 023): marca por user_id + revoked_at
// con scope='user' que el gate coteja contra el iat del token — queda
// revocado todo token EMITIDO ANTES del corte; los emitidos después siguen
// válidos. El logout de una sola sesión (marca scope='jti') NO cruza al
// predicado por usuario (criterio (c) de T8i), y un token legacy sin iat
// queda revocado por cualquier marca viva (fail-closed).
func TestRLS_RevokeAllCortePorUsuario(t *testing.T) {
	ts := newTestRLSServer(t)
	base := ts.Server.URL + "/api/v1"

	// tokenPreCut es ts.TokenA: emitido en el setup, ANTES de cualquier corte.
	tokenPreCut := ts.TokenA

	// Helper local: access token de A/B con iat controlado. El corte se coteja
	// por iat, así que cada subtest firma el token con la emisión que prueba.
	makeTokenConIat := func(userID, tenantID uuid.UUID, email string, issuedAt time.Time) string {
		t.Helper()
		claims := value_object.NewTokenClaims(
			userID, tenantID, ts.RoleID, email, testNamespace,
			value_object.DefaultTenantFeatures(),
			issuedAt.Add(15*time.Minute),
		)
		claims.Roles = []string{"tenant_admin"}
		claims.IssuedAt = issuedAt.Unix()
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, adapter.JWTClaims{TokenClaims: *claims})
		tokenStr, err := token.SignedString([]byte(testJWTSecret))
		require.NoError(t, err)
		return tokenStr
	}

	var corte time.Time

	t.Run("control positivo: un token emitido antes del corte autoriza antes de revoke-all", func(t *testing.T) {
		// El control que distingue "el gate funciona" de "todo da 401": sin
		// corte previo, el token del usuario autoriza gestión.
		resp := get(t, base+"/users", tokenPreCut, map[string]string{"X-Tenant-ID": ts.TenantA.String()})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("POST /auth/revoke-all responde OK", func(t *testing.T) {
		resp := postJSON(t, base+"/auth/revoke-all", tokenPreCut, map[string]string{})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		// El corte exacto es el revoked_at de la marca en la DB (no un reloj
		// aproximado): los subtests siguientes firman tokens alrededor de él.
		require.NoError(t, ts.SuperDB.QueryRow(
			`SELECT revoked_at FROM revoked_tokens WHERE user_id = $1 AND scope = 'user' ORDER BY revoked_at DESC LIMIT 1`,
			ts.UserA).Scan(&corte),
			"revoke-all debe haber insertado la marca scope='user'")
	})

	t.Run("la marca de alcance user quedó en revoked_tokens", func(t *testing.T) {
		var marcas int
		require.NoError(t, ts.SuperDB.QueryRow(
			`SELECT COUNT(*) FROM revoked_tokens WHERE user_id = $1 AND scope = 'user'`,
			ts.UserA).Scan(&marcas),
			"revoke-all debe insertar una marca scope='user' (migración 023), no un JTI decorativo")
		assert.GreaterOrEqual(t, marcas, 1)
	})

	t.Run("el access token emitido ANTES del corte recibe 401 (criterio (a) de T8i)", func(t *testing.T) {
		// El defecto original: este request devolvía 200 — el JTI sembrado por
		// revoke-all jamás matcheaba el del token presentado.
		resp := get(t, base+"/users", tokenPreCut, map[string]string{"X-Tenant-ID": ts.TenantA.String()})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"un token emitido antes de revoke-all debe quedar revocado por la marca de usuario")
	})

	t.Run("un token emitido DESPUÉS del corte sigue autorizando", func(t *testing.T) {
		// Sin este control, "todo da 401" sería indistinguible de aislamiento
		// correcto: la marca es un CORTE (revoked_at > iat), no un veto al
		// usuario entero — el usuario debe poder volver a loguearse.
		tokenPost := makeTokenConIat(ts.UserA, ts.TenantA, "admin-a@example.com", corte.Add(5*time.Second))
		resp := get(t, base+"/users", tokenPost, map[string]string{"X-Tenant-ID": ts.TenantA.String()})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("el corte de A no alcanza a B (RLS + predicado por user_id)", func(t *testing.T) {
		resp := get(t, base+"/users", ts.TokenB, map[string]string{"X-Tenant-ID": ts.TenantB.String()})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode,
			"la marca scope='user' de A no debe revocar tokens de otro usuario")
	})

	t.Run("logout de una sola sesión NO corta los demás tokens (criterio (c))", func(t *testing.T) {
		// Dos tokens de A emitidos después del corte. El logout de uno (marca
		// scope='jti' con revoked_at posterior al iat del otro) NO debe
		// revocarlo: sin el discriminador scope, un predicado por user_id +
		// revoked_at convertiría el logout en un revoke-all implícito.
		otro := makeTokenConIat(ts.UserA, ts.TenantA, "admin-a@example.com", corte.Add(10*time.Second))

		respLogout := postJSON(t, base+"/auth/logout", otro, map[string]string{})
		defer respLogout.Body.Close()
		assert.Equal(t, http.StatusNoContent, respLogout.StatusCode,
			"el logout de una sola sesión sigue funcionando")

		resp := get(t, base+"/users", otro, map[string]string{"X-Tenant-ID": ts.TenantA.String()})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"el JTI del token logueado queda revocado")

		tercero := makeTokenConIat(ts.UserA, ts.TenantA, "admin-a@example.com", corte.Add(10*time.Second))
		resp3 := get(t, base+"/users", tercero, map[string]string{"X-Tenant-ID": ts.TenantA.String()})
		defer resp3.Body.Close()
		assert.Equal(t, http.StatusOK, resp3.StatusCode,
			"la marca scope='jti' del logout no debe revocar otro token emitido antes del logout")
	})

	t.Run("token legacy sin iat queda revocado por la marca viva (fail-closed)", func(t *testing.T) {
		// Un token sin iat (legacy) no puede probar cuándo fue emitido: tras
		// revoke-all, cualquier marca user viva del usuario lo revoca.
		legacy := makeLegacyAccessToken(t, ts.UserA, ts.RoleID, "admin-a@example.com")
		resp := get(t, base+"/users", legacy, map[string]string{"X-Tenant-ID": ts.TenantA.String()})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("token legacy SIN marca de usuario sigue autorizando", func(t *testing.T) {
		// El fail-closed del iat=0 aplica sólo donde existe un corte: B no
		// tiene marca scope='user' y su token legacy (sin iat) sigue válido.
		legacyB := makeLegacyAccessToken(t, ts.UserB, ts.RoleID, "admin-b@example.com")
		resp := get(t, base+"/users", legacyB, map[string]string{"X-Tenant-ID": ts.TenantB.String()})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("el \"Ojo\" de la tarea: el índice cubre el predicado nuevo del gate", func(t *testing.T) {
		// El gate pasa a evaluar un predicado por user_id en CADA request. El
		// plan debe usar los índices (pkey para jti, idx_revoked_tokens_user_scope
		// parcial para la marca de usuario), no un Seq Scan. enable_seqscan=off
		// fuerza la visibilidad del camino por índice en una tabla de prueba
		// con pocas filas, donde el planner elegiría seq scan por costo.
		var existeIndice int
		require.NoError(t, ts.SuperDB.QueryRow(
			`SELECT COUNT(*) FROM pg_indexes WHERE indexname = 'idx_revoked_tokens_user_scope'`).
			Scan(&existeIndice))
		assert.Equal(t, 1, existeIndice, "el índice parcial de la 023 debe existir")

		var existeConstraint int
		require.NoError(t, ts.SuperDB.QueryRow(
			`SELECT COUNT(*) FROM pg_constraint WHERE conrelid = 'revoked_tokens'::regclass AND conname = 'revoked_tokens_scope_check'`).
			Scan(&existeConstraint))
		assert.Equal(t, 1, existeConstraint, "el CHECK de scope debe existir")

		ctx := context.Background()
		tx, err := ts.SuperDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer tx.Rollback()
		_, err = tx.Exec("SET LOCAL enable_seqscan = off")
		require.NoError(t, err)

		rows, err := tx.Query(`
			EXPLAIN SELECT EXISTS(
				SELECT 1 FROM revoked_tokens
				WHERE jti = $1
				   OR (scope = 'user' AND user_id = $2 AND revoked_at > to_timestamp($3))
			)`, uuid.New(), ts.UserA, time.Now().Unix())
		require.NoError(t, err)
		defer rows.Close()
		plan := ""
		for rows.Next() {
			var line string
			require.NoError(t, rows.Scan(&line))
			plan += line + "\n"
		}
		require.NoError(t, rows.Err())

		assert.Contains(t, plan, "BitmapOr", "el OR del gate debe resolverse por índices:\n%s", plan)
		assert.Contains(t, plan, "idx_revoked_tokens_user_scope", "la marca de usuario debe resolverse por el índice parcial:\n%s", plan)
	})
}
