//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	authmw "iam/src/auth/infrastructure/middleware"
	"iam/src/auth/infrastructure/s2s"
	planConfig "iam/src/plan/infrastructure/config"
	roleConfig "iam/src/role/infrastructure/config"
	"iam/src/shared/validator"
	tenantConfig "iam/src/tenant/infrastructure/config"
	userConfig "iam/src/user/infrastructure/config"
)

const (
	testNamespace     = "mc"
	keyWhatsappAgent  = "wa-key-16bytes-minimum-for-whatsapp-agent"
	keyTenantAdminSvc = "ta-key-16bytes-minimum-for-tenant-admin-svc"
	keySystemAdminSvc = "sa-key-16bytes-minimum-for-system-admin-svc"
)

// defaultTestPolicy expone la política S2S usada por todos los tests de integración.
// whatsapp-agent solo puede provisionar tenants; tenant-admin-svc administra
// users/roles tenant-scoped; system-admin-svc tiene acceso global legacy.
var defaultTestPolicy = map[string][]s2s.Scope{
	"whatsapp-agent":   {s2s.ScopeTenantProvision},
	"tenant-admin-svc": {s2s.ScopeTenantAdmin},
	"system-admin-svc": {s2s.ScopeSystemAdmin},
}

// testS2SServer levanta PostgreSQL + migraciones + router IAM con S2S auth.
type testS2SServer struct {
	Server *httptest.Server
	DB     *sql.DB
}

func newTestS2SServer(t *testing.T) *testS2SServer {
	t.Helper()

	ctx := context.Background()
	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("iam_test"),
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

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := sql.Open("postgres", connStr)
	require.NoError(t, err)
	require.NoError(t, db.PingContext(ctx))

	runMigrationsForS2S(t, db)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery())

	// Los tests de integración no arrancan por main(), así que registramos
	// manualmente los validadores custom (slug, etc.).
	validator.RegisterCustomValidators()

	// Sobrescribimos la política para este test: whatsapp-agent provision, tenant-admin-svc tenant:admin, system-admin-svc system:admin.
	// Nota: ServicePolicy es var, no const, así que podemos modificarla en test y restaurar.
	oldPolicy := s2s.ServicePolicy
	s2s.ServicePolicy = map[string][]s2s.Scope{
		"whatsapp-agent":   {s2s.ScopeTenantProvision},
		"tenant-admin-svc": {s2s.ScopeTenantAdmin},
		"system-admin-svc": {s2s.ScopeSystemAdmin},
	}
	t.Cleanup(func() { s2s.ServicePolicy = oldPolicy })

	// ⚠️ El registry se construye DESPUÉS de setear ServicePolicy, porque carga solo
	// los servicios que estén en la política y tengan key en el map.
	registry := s2s.LoadFromEnvForTests(map[string]string{
		"whatsapp-agent":   keyWhatsappAgent,
		"tenant-admin-svc": keyTenantAdminSvc,
		"system-admin-svc": keySystemAdminSvc,
	})

	authFactory := authmw.NewScopeMiddlewareFactory("jwt-secret-for-tests-only", testNamespace, registry)

	apiV1 := router.Group("/api/v1")
	adminGroup := apiV1.Group("", authFactory.RequireScope(s2s.ScopeSystemAdmin, "system_admin"))
	tenantScopedGroup := apiV1.Group("", authFactory.RequireScopes([]s2s.Scope{s2s.ScopeSystemAdmin, s2s.ScopeTenantAdmin}, "tenant_admin", "system_admin"))

	userFinderService := userConfig.SetupUserModule(tenantScopedGroup, db)
	tenantConfig.SetupTenantScopedModule(tenantScopedGroup, db, noopMetricsRecorder{})
	tenantFeaturesUC := tenantConfig.SetupTenantModule(adminGroup, db, noopMetricsRecorder{})
	_ = userFinderService
	_ = tenantFeaturesUC
	planConfig.SetupPlanModule(adminGroup, db)
	roleConfig.SetupRoleModule(tenantScopedGroup, db)

	// Grupo de provision: POST /tenants con scope tenant:provision o system:admin.
	provisionGroup := apiV1.Group("", authFactory.RequireScopes([]s2s.Scope{s2s.ScopeTenantProvision, s2s.ScopeSystemAdmin}, "system_admin"))
	tenantConfig.SetupTenantProvisionModule(provisionGroup, db, noopMetricsRecorder{})

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	return &testS2SServer{Server: srv, DB: db}
}

func runMigrationsForS2S(t *testing.T, db *sql.DB) {
	t.Helper()
	migrationsDir := findMigrationsDirForS2S(t)
	entries, err := os.ReadDir(migrationsDir)
	require.NoError(t, err)

	var sqlFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".up.sql") {
			sqlFiles = append(sqlFiles, filepath.Join(migrationsDir, entry.Name()))
		}
	}
	sort.Strings(sqlFiles)

	for _, f := range sqlFiles {
		content, err := os.ReadFile(f)
		require.NoError(t, err)
		_, err = db.Exec(string(content))
		require.NoError(t, err)
	}
}

func findMigrationsDirForS2S(t *testing.T) string {
	t.Helper()
	candidates := []string{"../migrations", "migrations"}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	t.Fatalf("could not find migrations directory")
	return ""
}

func s2sPostJSON(t *testing.T, url, key string, body interface{}) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("X-API-Key", key)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func s2sGet(t *testing.T, url, key string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	if key != "" {
		req.Header.Set("X-API-Key", key)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func s2sPutJSON(t *testing.T, url, key string, body interface{}) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(b))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("X-API-Key", key)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func TestS2S_WhatsappAgent_CanCreateTenant(t *testing.T) {
	srv := newTestS2SServer(t)
	url := fmt.Sprintf("%s/api/v1/tenants", srv.Server.URL)

	body := map[string]interface{}{
		"name":        "WA Tenant",
		"slug":        "wa-tenant-" + uuid.New().String()[:8],
		"description": "Tenant creado por whatsapp-agent",
		"type":        "STARTUP",
		"owner_id":    uuid.New().String(),
	}
	resp := s2sPostJSON(t, url, keyWhatsappAgent, body)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestS2S_WhatsappAgent_CannotAccessUsers(t *testing.T) {
	srv := newTestS2SServer(t)
	url := fmt.Sprintf("%s/api/v1/users", srv.Server.URL)

	resp := s2sGet(t, url, keyWhatsappAgent)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestS2S_WhatsappAgent_CannotListTenants(t *testing.T) {
	srv := newTestS2SServer(t)
	url := fmt.Sprintf("%s/api/v1/tenants", srv.Server.URL)

	resp := s2sGet(t, url, keyWhatsappAgent)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestS2S_TenantAdmin_CanAccessUsers(t *testing.T) {
	srv := newTestS2SServer(t)
	// Listar users requiere X-Tenant-ID; creamos un tenant para obtener uno válido.
	tenantID := createTenantWithS2S(t, srv, "TA Users Tenant", "ta-users-"+uuid.New().String()[:8])
	url := fmt.Sprintf("%s/api/v1/users", srv.Server.URL)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", keyTenantAdminSvc)
	req.Header.Set("X-Tenant-ID", tenantID)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestS2S_TenantAdmin_CannotCreateTenant(t *testing.T) {
	srv := newTestS2SServer(t)
	url := fmt.Sprintf("%s/api/v1/tenants", srv.Server.URL)

	body := map[string]interface{}{
		"name":        "TA Tenant",
		"slug":        "ta-tenant-" + uuid.New().String()[:8],
		"description": "Intento de crear tenant sin scope",
		"type":        "STARTUP",
		"owner_id":    uuid.New().String(),
	}
	resp := s2sPostJSON(t, url, keyTenantAdminSvc, body)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestS2S_UnknownKey_FallsBackTo401(t *testing.T) {
	srv := newTestS2SServer(t)
	url := fmt.Sprintf("%s/api/v1/tenants", srv.Server.URL)

	resp := s2sGet(t, url, "totally-unknown-key")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestS2S_SystemAdmin_CanCreateTenantAndAccessUsers(t *testing.T) {
	srv := newTestS2SServer(t)
	base := fmt.Sprintf("%s/api/v1", srv.Server.URL)

	// Crear tenant como system-admin y luego listar users con X-Tenant-ID del tenant creado.
	tenantID := createTenantWithS2S(t, srv, "SA Tenant", "sa-tenant-"+uuid.New().String()[:8])

	url := base + "/users"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", keySystemAdminSvc)
	req.Header.Set("X-Tenant-ID", tenantID)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestS2S_CrossTenant_IsolationViaUserFilter(t *testing.T) {
	srv := newTestS2SServer(t)
	base := fmt.Sprintf("%s/api/v1", srv.Server.URL)

	// Crear dos tenants usando system-admin-svc.
	tenantA := createTenantWithS2S(t, srv, "Tenant A", "tenant-a-"+uuid.New().String()[:8])
	tenantB := createTenantWithS2S(t, srv, "Tenant B", "tenant-b-"+uuid.New().String()[:8])

	// Crear un user en cada tenant con tenant-admin-svc.
	userInA := createUserWithS2S(t, srv, tenantA, "user-a@example.com")
	_ = createUserWithS2S(t, srv, tenantB, "user-b@example.com")

	// Listar users de tenant A con X-Tenant-ID = tenantA. Debe devolver solo user-a.
	url := base + "/users"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", keyTenantAdminSvc)
	req.Header.Set("X-Tenant-ID", tenantA)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
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
	assert.Contains(t, emails, "user-a@example.com")
	assert.NotContains(t, emails, "user-b@example.com")

	// Intentar obtener user de tenant A usando tenant B en header: debe devolver 404.
	getURL := fmt.Sprintf("%s/users/%s", base, userInA)
	req2, err := http.NewRequest(http.MethodGet, getURL, nil)
	require.NoError(t, err)
	req2.Header.Set("X-API-Key", keyTenantAdminSvc)
	req2.Header.Set("X-Tenant-ID", tenantB)
	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp2.StatusCode)
}

func createTenantWithS2S(t *testing.T, srv *testS2SServer, name, slug string) string {
	t.Helper()
	url := fmt.Sprintf("%s/api/v1/tenants", srv.Server.URL)
	body := map[string]interface{}{
		"name":        name,
		"slug":        slug,
		"description": name,
		"type":        "STARTUP",
		"owner_id":    uuid.New().String(),
	}
	resp := s2sPostJSON(t, url, keySystemAdminSvc, body)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var tenant tenantResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&tenant))
	return tenant.ID
}

func createUserWithS2S(t *testing.T, srv *testS2SServer, tenantID, email string) string {
	t.Helper()
	url := fmt.Sprintf("%s/api/v1/users", srv.Server.URL)
	body := map[string]interface{}{
		"email":      email,
		"password":   "StrongP@ssw0rd",
		"first_name": "Test",
		"last_name":  "User",
		"tenant_id":  tenantID,
		"role_id":    uuid.New().String(),
		"status":     "ACTIVE",
	}
	resp := s2sPostJSON(t, url, keyTenantAdminSvc, body)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var user map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&user))
	return user["id"].(string)
}
