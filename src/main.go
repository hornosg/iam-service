package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/hornosg/go-shared/infrastructure/env"
	tenantmw "github.com/hornosg/go-shared/infrastructure/middleware"
	"github.com/hornosg/go-shared/infrastructure/postgres"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"iam/src/auth/infrastructure/adapter"
	"iam/src/auth/infrastructure/config"
	authmw "iam/src/auth/infrastructure/middleware"
	"iam/src/auth/infrastructure/s2s"
	planConfig "iam/src/plan/infrastructure/config"
	roleConfig "iam/src/role/infrastructure/config"
	tenantConfig "iam/src/tenant/infrastructure/config"
	userConfig "iam/src/user/infrastructure/config"
	sharedpostgres "iam/src/shared/postgres"
	"iam/src/shared/validator"
	userRepo "iam/src/user/infrastructure/persistence/repository"
	userUC "iam/src/user/application/usecase"

	sharedport "github.com/hornosg/go-shared/domain/port"
	sharedlog "github.com/hornosg/go-shared/infrastructure/logging"
	sharedmetrics "github.com/hornosg/go-shared/infrastructure/metrics"
	sharedmigrate "github.com/hornosg/go-shared/migrate"

	iamroot "iam"
)

func init() {
	validator.RegisterCustomValidators()
}

func main() {
	// Configuración de la base de datos: dos pools según ACC-E02 T2/T5.
	//   * appDB: account_app — rol de aplicación con RLS (todo caso de uso
	//     con tenant conocido y operaciones post-auth del login).
	//   * loginDB: iam_login — rol acotado de pre-auth; sólo resuelve
	//     credenciales sin filtro de tenant (T1-D1, T1-D2).
	appDB, loginDB, err := setupDatabases()
	if err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}
	defer appDB.Close()
	defer loginDB.Close()

	// Fail-fast anti-superuser (ACC-E02 T6, patrón PLAT-E29 T7 / RULE-09/RULE-10): el runtime
	// NUNCA debe correr como superuser/BYPASSRLS. FORCE ROW LEVEL SECURITY no aplica a superusers →
	// con un rol privilegiado la RLS de users, tenants, refresh_tokens y revoked_tokens queda inerte:
	// el servicio serviría datos cross-tenant sin error visible. Se verifica en AMBOS pools (T1-D2:
	// dos pools reales, appDB con account_app y loginDB con iam_login; ambos deben ser NOBYPASSRLS)
	// y, desde T8n, también sobre la conexión de migraciones (account_migrator): el conteo de
	// guards es el de conexiones abiertas.
	if err := sharedpostgres.AssertNoRLSBypass(appDB); err != nil {
		log.Fatalf("%v", err)
	}
	if err := sharedpostgres.AssertNoRLSBypass(loginDB); err != nil {
		log.Fatalf("%v", err)
	}

	// Migraciones versionadas in-app (ADR-001) — fail-fast antes de servir tráfico.
	// ACC-E02 T8n: corren con el rol dedicado account_migrator (DDL sobre iam_db, SIN
	// uso en runtime), en una conexión propia que se cierra al terminar de migrar —
	// NO sobre el pool de aplicación: account_app sólo tiene SELECT sobre
	// schema_migrations (rol de menor privilegio, correcto para runtime), así que
	// todo arranque con una migración pendiente fallaba con permission denied y el
	// workaround era el baile de dos arranques con DB_USER=postgres +
	// ALLOW_SUPERUSER_DB=true. Bootstrap del rol: scripts/bootstrap_migrator.sh.
	migrateDB, err := setupMigratorDatabase()
	if err != nil {
		log.Fatalf("Error connecting migration role: %v", err)
	}
	// El guard de T6 no se relaja con el tercer rol: tercera conexión abierta →
	// tercer guard. Un rol SUPERUSER/BYPASSRLS en DB_MIGRATE_USER migraría con la
	// RLS inerte y se rechaza igual que en runtime.
	if err := sharedpostgres.AssertNoRLSBypass(migrateDB); err != nil {
		log.Fatalf("%v", err)
	}
	dbName := env.Get("DB_NAME", "iam_db")
	if err := sharedmigrate.RunMigrations(migrateDB, iamroot.MigrationsFS, dbName); err != nil {
		log.Fatalf("Error running migrations: %v", err)
	}
	// Cerrada al terminar de migrar (decisión del owner, T8n): no es un pool de
	// servicio y jamás corre queries de negocio.
	if err := migrateDB.Close(); err != nil {
		log.Printf("cierre de la conexión de migraciones: %v", err)
	}

	// Configuración del router
	router := gin.New() // Usar gin.New() para evitar middlewares duplicados

	// Agregar middlewares básicos necesarios
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// Validación de tenant (X-Tenant-ID vs JWT tenant_id)
	securityLogger := sharedlog.NewSecurityLogger("iam")
	serviceNamespace := env.Get("SERVICE_NAMESPACE", "mc")
	router.Use(tenantmw.TenantValidation(tenantmw.TenantValidationConfig{
		JWTSecret: os.Getenv("JWT_SECRET"),
		Namespace: serviceNamespace,
		ExcludedRoutes: []string{
			"/health",
			"/api/v1/health",
			"/metrics",
			"/api/v1/auth/*",
			"/api/v1/tenants*",
			"/api/v1/users*",
			"/api/v1/roles*",
			"/api/v1/plans*",
		},
		OnTenantMismatch: func(userID, jwtTenantID, headerTenantID, ipAddress string) {
			securityLogger.Log(sharedport.SecurityEvent{
				Event:          sharedport.EventTenantMismatch,
				UserID:         userID,
				JWTTenantID:    jwtTenantID,
				HeaderTenantID: headerTenantID,
				IPAddress:      ipAddress,
			})
		},
		OnNamespaceMismatch: func(userID, jwtNamespace, expectedNamespace, ipAddress string) {
			securityLogger.Log(sharedport.SecurityEvent{
				Event:     sharedport.EventTenantMismatch,
				UserID:    userID,
				IPAddress: ipAddress,
				Reason:    "namespace_mismatch: jwt=" + jwtNamespace + " expected=" + expectedNamespace,
			})
		},
	}))

	// Configurar Prometheus metrics si está habilitado
	prometheusEnabled := os.Getenv("PROMETHEUS_ENABLED")
	log.Printf("PROMETHEUS_ENABLED value: '%s'", prometheusEnabled)

	if prometheusEnabled == "true" {
		log.Println("Registering /metrics endpoint")
		// Endpoint de métricas usando la librería oficial de Prometheus
		router.GET("/metrics", gin.WrapH(promhttp.Handler()))
		log.Println("/metrics endpoint registered successfully")
	} else {
		log.Println("Prometheus metrics disabled")
	}

	// Configuración de CORS
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "up",
			"service": "iam",
		})
	})

	// API v1 group
	apiV1 := router.Group("/api/v1")

	// Shared infrastructure
	metricsRecorder := sharedmetrics.NewPrometheusRecorder()

	// Gates de acceso a los endpoints de gestión. Cierran el agujero del baseline
	// de Kong: estos endpoints confiaban en el gateway, cuyo fallback anónimo los
	// dejaba abiertos (ej. GET /api/v1/tenants sin token → 200). Servicios S2S
	// autorizan por X-API-Key + scope; humanos por JWT + rol.
	//   - adminGroup       (cross-tenant global): plans, roles (escritura) → system:admin
	//   - tenantScopedGroup (tenant-scoped):      users, tenants/:id, roles (lectura) → system:admin or tenant:admin
	//
	// El registro S2S carga una credencial por servicio consumidor desde
	// S2S_KEY_<SERVICE>. Política de scopes vive en código (s2s.ServicePolicy).
	// Si ninguna key de env está presente, el registro queda vacío: S2S falla
	// closed (igual que antes si no había S2S_API_KEY).
	s2sRegistry, err := s2s.LoadFromEnv()
	if err != nil {
		log.Fatalf("Error loading S2S registry: %v", err)
	}
	jwtSecret := os.Getenv("JWT_SECRET")

	// Gate de revocación de tokens — ACC-E02 T8g (criterio (a)): DEBE registrarse
	// sobre apiV1 ANTES de crear adminGroup/tenantScopedGroup/provisionGroup. Gin
	// congela la cadena de handlers de un grupo en el momento de crearlo; el
	// registro posterior de un Use() sobre el padre (como hacía SetupAuthModule)
	// no los alcanzaba y un JTI revocado seguía autorizando /users, /tenants/:id,
	// /roles y /plans hasta la expiración por tiempo del access token.
	// Devuelve el repo de auth sobre account_app que comparte con SetupAuthModule.
	authRepoApp := config.SetupTokenRevocationGate(apiV1, appDB, loginDB, jwtSecret)

	authFactory := authmw.NewScopeMiddlewareFactory(jwtSecret, serviceNamespace, s2sRegistry)
	adminGroup := apiV1.Group("", authFactory.RequireScope(s2s.ScopeSystemAdmin, "system_admin"))
	tenantScopedGroup := apiV1.Group("", authFactory.RequireScopes([]s2s.Scope{s2s.ScopeSystemAdmin, s2s.ScopeTenantAdmin}, "tenant_admin", "system_admin"))

	// Configurar módulos en orden de dependencias
	// 1. User Module (independiente) - retorna UserFinderService con account_app.
	userFinderService := userConfig.SetupUserModule(tenantScopedGroup, appDB)

	// User finder para la fase pre-auth del login (iam_login). No registra rutas;
	// sólo se inyecta en el LoginUseCase.
	loginUserRepo := userRepo.NewPostgresUserRepository(loginDB)
	loginUserFinder := userUC.NewUserFinderUseCase(loginUserRepo)

	// 2. Tenant Management Module (tenant-scoped): lectura/escritura por ID
	//    GET/PUT/DELETE /tenants/:id se mueven al grupo tenant-scoped para que
	//    servicios como onboarding (tenant:admin) puedan gestionar sus propios
	//    tenants sin necesitar system:admin. List/Plan/Features quedan en adminGroup.
	tenantConfig.SetupTenantScopedModule(tenantScopedGroup, appDB, metricsRecorder)

	// 3. Tenant Admin Module (cross-tenant global): list, plans, features → system:admin
	tenantFeaturesUC := tenantConfig.SetupTenantModule(adminGroup, appDB, metricsRecorder)

	// 4. Auth Module (depende de User y Tenant)
	// El adapter convierte tenant_vo.TenantFeatures → auth_vo.TenantFeatures (anti-corruption layer)
	tenantService := adapter.NewTenantFeaturesAdapter(tenantFeaturesUC)
	authConfig := config.NewAuthModuleConfigFromEnv()
	config.SetupAuthModule(apiV1, appDB, loginDB, authRepoApp, userFinderService, loginUserFinder, tenantService, authConfig)

	// 5. Plan Module (independiente)
	planConfig.SetupPlanModule(adminGroup, appDB)

	// 6. Role Module (catálogo global, ACC-E02 T10). `roles` no lleva RLS ni
	//    tenant_id: el gate de scope es la única defensa de la tabla. Las rutas
	//    de lectura (GET /roles, GET /roles/:id) quedan en tenantScopedGroup
	//    (legibles por system:admin o tenant:admin); las de escritura
	//    (POST/PUT/DELETE) pasan a adminGroup (system:admin únicamente), cerrando
	//    la escalada por la que un tenant:admin podía crearse un rol SYSTEM_ADMIN
	//    y mutar los roles de sistema existentes.
	roleConfig.SetupRoleModule(tenantScopedGroup, adminGroup, appDB)

	// 7. Tenant Provision Module — SOLO POST /tenants para whatsapp-agent/onboarding con scope tenant:provision.
	// También permitimos system:admin (es un super-scope) para no forzar a sales
	// a tener una key separada de tenant:provision mientras migran.
	provisionGroup := apiV1.Group("", authFactory.RequireScopes([]s2s.Scope{s2s.ScopeTenantProvision, s2s.ScopeSystemAdmin}, "system_admin"))
	tenantConfig.SetupTenantProvisionModule(provisionGroup, appDB, metricsRecorder)

	// Iniciar el servidor
	port := env.Get("PORT", "8080")
	log.Printf("Starting IAM server on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}

func setupDatabases() (appDB *sql.DB, loginDB *sql.DB, err error) {
	// Configuración compartida de la base de datos desde variables de entorno.
	host := env.Get("DB_HOST", "localhost")
	port := env.Get("DB_PORT", "5432")
	// ACC-E02 T5: cada pool tiene su propia credencial. Un único password
	// compartido haría que comprometer iam_login (pre-auth) entregue account_app.
	appPassword := env.Get("DB_PASSWORD", "lab_account_app")
	loginPassword := env.Get("DB_LOGIN_PASSWORD", "lab_iam_login")
	dbname := env.Get("DB_NAME", "iam_db")
	sslmode := env.Get("DB_SSLMODE", "disable")

	// Pool de aplicación: account_app (RLS, todo excepto lookup de credencial).
	appUser := env.Get("DB_USER", "account_app")
	appDB, err = postgres.Connect(postgres.Config{
		Host:     host,
		Port:     port,
		User:     appUser,
		Password: appPassword,
		DBName:   dbname,
		SSLMode:  sslmode,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("connect account_app: %w", err)
	}
	postgres.StartPoolMonitor(context.Background(), appDB, postgres.MonitorOptions{
		Service: "iam-service",
		DBName:  dbname,
	})

	// Pool de login: iam_login (sólo resuelve credenciales pre-auth).
	loginUser := env.Get("DB_LOGIN_USER", "iam_login")
	loginDB, err = postgres.Connect(postgres.Config{
		Host:     host,
		Port:     port,
		User:     loginUser,
		Password: loginPassword,
		DBName:   dbname,
		SSLMode:  sslmode,
	})
	if err != nil {
		_ = appDB.Close()
		return nil, nil, fmt.Errorf("connect iam_login: %w", err)
	}
	postgres.StartPoolMonitor(context.Background(), loginDB, postgres.MonitorOptions{
		Service: "iam-service-login",
		DBName:  dbname,
	})

	log.Printf("Successfully connected to database as app=%s login=%s", appUser, loginUser)
	return appDB, loginDB, nil
}

// setupMigratorDatabase abre la conexión dedicada de migraciones (ACC-E02 T8n): rol
// account_migrator con privilegio de DDL sobre iam_db y SIN uso en runtime. Es una
// conexión, no un pool de servicio: no lleva monitor de métricas (viviría lo que
// dura el boot) y se cierra en main() apenas termina RunMigrations. La credencial
// sale de DB_MIGRATE_USER / DB_MIGRATE_PASSWORD y NO comparte secreto con los otros
// roles — misma decisión de T5: una credencial comprometida no entrega las demás.
func setupMigratorDatabase() (*sql.DB, error) {
	host := env.Get("DB_HOST", "localhost")
	port := env.Get("DB_PORT", "5432")
	migrateUser := env.Get("DB_MIGRATE_USER", "account_migrator")
	migratePassword := env.Get("DB_MIGRATE_PASSWORD", "lab_account_migrator")
	dbname := env.Get("DB_NAME", "iam_db")
	sslmode := env.Get("DB_SSLMODE", "disable")

	db, err := postgres.Connect(postgres.Config{
		Host:     host,
		Port:     port,
		User:     migrateUser,
		Password: migratePassword,
		DBName:   dbname,
		SSLMode:  sslmode,
	})
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", migrateUser, err)
	}

	log.Printf("Successfully connected to database as migrator=%s", migrateUser)
	return db, nil
}

