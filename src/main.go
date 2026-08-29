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

	// Migraciones versionadas in-app (ADR-001) — fail-fast antes de servir tráfico.
	// Corren sobre el rol de aplicación; las migraciones 017/018/019 son idempotentes.
	dbName := env.Get("DB_NAME", "iam_db")
	if err := sharedmigrate.RunMigrations(appDB, iamroot.MigrationsFS, dbName); err != nil {
		log.Fatalf("Error running migrations: %v", err)
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
	config.SetupAuthModule(apiV1, appDB, loginDB, userFinderService, loginUserFinder, tenantService, authConfig)

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
