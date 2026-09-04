package main

// buildRouter construye el router completo del servicio con todos sus módulos
// montados. ACC-E01 T8: extracción mecánica desde main.go — el bootstrap se
// adelgaza a la conexión de DBs, guards y migraciones, y el test de golden de
// rutas (router_test.go / routes.golden) pasa a poder cablear el router con
// DSNs dummy (sql.Open no conecta) sin levantar infraestructura.
//
// El orden de montaje es parte del contrato: el gate de revocación (T8g)
// DEBE registrarse sobre apiV1 antes de crear adminGroup/tenantScopedGroup.

import (
	"database/sql"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/hornosg/go-shared/infrastructure/env"
	tenantmw "github.com/hornosg/go-shared/infrastructure/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	sharedport "github.com/hornosg/go-shared/domain/port"
	sharedlog "github.com/hornosg/go-shared/infrastructure/logging"
	sharedmetrics "github.com/hornosg/go-shared/infrastructure/metrics"

	"iam/src/auth/infrastructure/adapter"
	"iam/src/auth/infrastructure/config"
	authmw "iam/src/auth/infrastructure/middleware"
	"iam/src/auth/infrastructure/s2s"
	planConfig "iam/src/plan/infrastructure/config"
	roleConfig "iam/src/role/infrastructure/config"
	tenantConfig "iam/src/tenant/infrastructure/config"
	userConfig "iam/src/user/infrastructure/config"
	userRepo "iam/src/user/infrastructure/persistence/repository"
	userUC "iam/src/user/application/usecase"
)

func buildRouter(appDB *sql.DB, loginDB *sql.DB) *gin.Engine {
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

	return router
}