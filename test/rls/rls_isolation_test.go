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

	s2sRegistry := s2s.LoadFromEnvForTests(map[string]string{})
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
	authconfig.SetupAuthModule(apiV1, appDB, loginDB, userFinderService, loginUserFinder, tenantService, authCfg)

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
}

// — Tokens de sesión (refresh_tokens / revoked_tokens): objeción (B) del gate
// L4 de T8b (2026-08-31), encuadrada en T8. Las policies de estas dos tablas
// son una subselect `user_id IN (...)` — distintas de las de users/tenants —
// y merecen evidencia HTTP propia. Los controles positivos (A refresca el suyo,
// B refresca el suyo) distinguen aislamiento correcto de un fail-closed global,
// que es exactamente el modo de fallo que el checkpoint de T8 (2026-08-29)
// encontró en users/tenants antes de T8b.
func seedRefreshToken(t *testing.T, db *sql.DB, userID uuid.UUID, token string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO refresh_tokens (id, user_id, token, expires_at, created_at)
		VALUES ($1, $2, $3, $4, NOW())`,
		uuid.New(), userID, token, time.Now().Add(7*24*time.Hour))
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
	seedRefreshToken(t, ts.SuperDB, ts.UserA, "rt-a-propio")
	seedRefreshToken(t, ts.SuperDB, ts.UserA, "rt-a-para-b")
	seedRefreshToken(t, ts.SuperDB, ts.UserB, "rt-b-propio")
	seedRefreshToken(t, ts.SuperDB, ts.UserB, "rt-b-para-a")

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

	t.Run("logout de B responde 204", func(t *testing.T) {
		resp := postJSON(t, base+"/auth/logout", ts.TokenB, map[string]string{})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
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
}
