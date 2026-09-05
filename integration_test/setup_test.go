//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	sharedport "github.com/hornosg/go-shared/domain/port"
	authmw "github.com/hornosg/iam-service/src/access/infrastructure/middleware"
	"github.com/hornosg/iam-service/src/access/infrastructure/s2s"
	planConfig "github.com/hornosg/iam-service/src/plans/infrastructure/config"
	roleConfig "github.com/hornosg/iam-service/src/access/infrastructure/config"
	"github.com/hornosg/iam-service/src/shared/validator"
	tenantConfig "github.com/hornosg/iam-service/src/tenancy/infrastructure/config"
)

// defaultIntegrationPolicy y las claves S2S viven en s2s_auth_test.go (mismo
// paquete): los tests de integración comparten política y claves.

// noopMetricsRecorder es un MetricsRecorder que no hace nada. Útil para tests
// de integración que no necesitan emitir métricas reales.
type noopMetricsRecorder struct{}

func (noopMetricsRecorder) Record(_ sharedport.MetricEvent) {}

// testServer agrupa el servidor HTTP y la DB para los tests de integración.
type testServer struct {
	Server *httptest.Server
	DB     *sql.DB
}

// newTestServer levanta un contenedor PostgreSQL, ejecuta migraciones
// y retorna un httptest.Server con todos los módulos IAM configurados.
func newTestServer(t *testing.T) *testServer {
	t.Helper()

	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		// ACC-E02 T11: el contenedor usa el nombre `iam_db` porque las
		// migraciones 017/018 hardcodean `GRANT CONNECT ON DATABASE iam_db`
		// (carry-forward de T4; misma solución del paquete test/rls).
		postgres.WithDatabase("iam_db"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	if err != nil {
		t.Fatalf("error starting postgres container: %v", err)
	}

	t.Cleanup(func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("warn: failed to terminate postgres container: %v", err)
		}
	})

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("error getting connection string: %v", err)
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		t.Fatalf("error opening database: %v", err)
	}
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("error pinging database: %v", err)
	}

	runMigrations(t, db)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery())

	// Los tests de integración no arrancan por main(), así que registramos
	// manualmente los validadores custom (slug, etc.).
	validator.RegisterCustomValidators()

	// ACC-E02 T11 (nota D5 de T10): el módulo de roles se registra con los
	// grupos REALES del router de producción — lectura en tenantScopedGroup,
	// escritura en adminGroup — para que el integration test ejerza el gate de
	// scope por HTTP, no sólo el unit test de rutas.
	oldPolicy := s2s.ServicePolicy
	s2s.ServicePolicy = map[string][]s2s.Scope{
		"whatsapp-agent":   {s2s.ScopeTenantProvision},
		"tenant-admin-svc": {s2s.ScopeTenantAdmin},
		"system-admin-svc": {s2s.ScopeSystemAdmin},
	}
	t.Cleanup(func() { s2s.ServicePolicy = oldPolicy })

	registry := s2s.LoadFromEnvForTests(map[string]string{
		"whatsapp-agent":   keyWhatsappAgent,
		"tenant-admin-svc": keyTenantAdminSvc,
		"system-admin-svc": keySystemAdminSvc,
	})
	authFactory := authmw.NewScopeMiddlewareFactory("jwt-secret-for-tests-only", testNamespace, registry)

	apiV1 := router.Group("/api/v1")
	adminGroup := apiV1.Group("", authFactory.RequireScope(s2s.ScopeSystemAdmin, "system_admin"))
	tenantScopedGroup := apiV1.Group("", authFactory.RequireScopes([]s2s.Scope{s2s.ScopeSystemAdmin, s2s.ScopeTenantAdmin}, "tenant_admin", "system_admin"))

	tenantConfig.SetupTenantScopedModule(apiV1, db, noopMetricsRecorder{})
	tenantConfig.SetupTenantProvisionModule(apiV1, db, noopMetricsRecorder{})
	// GET /tenants (listado admin), plan y features viven en adminGroup en
	// producción (main.go); sin esto el integration test no ejercita el gate
	// del listado cross-tenant.
	tenantFeaturesUC := tenantConfig.SetupTenantModule(adminGroup, db, noopMetricsRecorder{})
	_ = tenantFeaturesUC
	roleConfig.SetupRoleModule(tenantScopedGroup, adminGroup, db)
	planConfig.SetupPlanModule(apiV1, db)

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	return &testServer{Server: srv, DB: db}
}

// runMigrations ejecuta todos los archivos .sql del directorio migrations en orden.
func runMigrations(t *testing.T, db *sql.DB) {
	t.Helper()

	// golang-migrate (go-shared/migrate → RunMigrations) crea esta tabla al
	// inicializar su driver; el runner crudo de integración la necesita
	// pre-creada porque la migración 018 le hace GRANT.
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version bigint NOT NULL PRIMARY KEY,
		dirty   boolean NOT NULL
	)`)
	if err != nil {
		t.Fatalf("error creating schema_migrations table: %v", err)
	}

	migrationsDir := findMigrationsDir(t)

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("error reading migrations dir %s: %v", migrationsDir, err)
	}

	var sqlFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".up.sql") {
			sqlFiles = append(sqlFiles, filepath.Join(migrationsDir, entry.Name()))
		}
	}
	sort.Strings(sqlFiles)

	for _, f := range sqlFiles {
		content, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("error reading migration %s: %v", f, err)
		}
		if _, err := db.Exec(string(content)); err != nil {
			t.Fatalf("error executing migration %s: %v", f, err)
		}
	}
}

// findMigrationsDir localiza el directorio migrations relativo a este archivo de test.
func findMigrationsDir(t *testing.T) string {
	t.Helper()

	// El test corre desde services/iam-service/integration_test/
	// Las migraciones están en services/iam-service/migrations/
	candidates := []string{
		"../migrations",
		"migrations",
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	t.Fatalf("could not find migrations directory")
	return ""
}

// baseURL construye la URL base del servidor de test.
func baseURL(srv *testServer) string {
	return fmt.Sprintf("%s/api/v1", srv.Server.URL)
}
