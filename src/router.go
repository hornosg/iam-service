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
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hornosg/go-shared/infrastructure/env"
	tenantmw "github.com/hornosg/go-shared/infrastructure/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/hornosg/go-shared/domain/service"
	sharedport "github.com/hornosg/go-shared/domain/port"
	sharedlog "github.com/hornosg/go-shared/infrastructure/logging"
	sharedmetrics "github.com/hornosg/go-shared/infrastructure/metrics"

	"github.com/hornosg/iam-service/src/identity/domain/port"
	"github.com/hornosg/iam-service/src/identity/infrastructure/adapter"
	"github.com/hornosg/iam-service/src/identity/infrastructure/config"
	authmw "github.com/hornosg/iam-service/src/access/infrastructure/middleware"
	"github.com/hornosg/iam-service/src/access/infrastructure/s2s"
	planConfig "github.com/hornosg/iam-service/src/plans/infrastructure/config"
	roleConfig "github.com/hornosg/iam-service/src/access/infrastructure/config"
	tenantConfig "github.com/hornosg/iam-service/src/tenancy/infrastructure/config"
	tenantUC "github.com/hornosg/iam-service/src/tenancy/application/usecase"
	userConfig "github.com/hornosg/iam-service/src/identity/infrastructure/config"
	userRepo "github.com/hornosg/iam-service/src/identity/infrastructure/persistence/repository"
	userUC "github.com/hornosg/iam-service/src/identity/application/usecase"
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

	// Switch de módulos — ACC-E01 T7: MODULES_DISABLED (lista separada por
	// comas) decide qué módulos NO se montan: su Setup*Module no corre y sus
	// rutas no existen en el router. Sin la variable, todos los módulos se
	// montan — el default es el comportamiento de hoy. El switch vive SOLO en
	// el wiring (este archivo), nunca dentro de los módulos. Ver modulesDisabled
	// al pie para las claves canónicas.
	disabled := modulesDisabled()

	// Dependencia del wiring: identity (login) consume tenancy (claim de tenant
	// vía SetupTenantModule → TenantFeaturesAdapter). Desmontar tenancy dejando
	// identity montado es configuración inconsistente: el login quedaría a
	// medias. El servicio se niega a arrancar antes que exponerlo roto en
	// silencio. (La condición de diseño de PROP-013 prohíbe la dependencia de
	// subscription, no la de tenancy — esa existe y es legítima.)
	if disabled["tenancy"] && !disabled["identity"] {
		log.Fatalf("MODULES_DISABLED: identity (login) consume tenancy — desmontá también identity para arrancar sin tenancy")
	}

	// Gate de revocación de tokens — ACC-E02 T8g (criterio (a)): DEBE registrarse
	// sobre apiV1 ANTES de crear adminGroup/tenantScopedGroup/provisionGroup. Gin
	// congela la cadena de handlers de un grupo en el momento de crearlo; el
	// registro posterior de un Use() sobre el padre (como hacía SetupAuthModule)
	// no los alcanzaba y un JTI revocado seguía autorizando /users, /tenants/:id,
	// /roles y /plans hasta la expiración por tiempo del access token.
	// Devuelve el repo de auth sobre account_app que comparte con SetupAuthModule.
	// ACC-E01 T7: parte del módulo identity — se monta con él.
	//
	// ACC-E03 T4: la config (que carga la clave de firma) y el verificador dual
	// se crean ANTES del gate de revocación, para que firmador, gate de
	// revocación y gate authorize (vía PublicKeyFor) compartan UNA sola
	// instancia y UNA keyring — la rotación (T8) agrega la clave en gracia una
	// sola vez. El orden con respecto a los grupos es el de T8g: el gate se
	// registra sobre apiV1 antes de crear los grupos de gestión.
	var authRepoApp port.AuthRepository
	var authConfig config.AuthModuleConfig
	var signingVerifier *adapter.JWTServiceAdapter
	if !disabled["identity"] {
		authConfig = config.NewAuthModuleConfigFromEnv()
		signingVerifier = adapter.NewJWTServiceAdapter(authConfig.JWTSecret, authConfig.SigningKey)
		authRepoApp = config.SetupTokenRevocationGate(apiV1, appDB, loginDB, signingVerifier)
	}

	authFactory := authmw.NewScopeMiddlewareFactory(jwtSecret, serviceNamespace, s2sRegistry, signingVerifier)
	adminGroup := apiV1.Group("", authFactory.RequireScope(s2s.ScopeSystemAdmin, "system_admin"))
	tenantScopedGroup := apiV1.Group("", authFactory.RequireScopes([]s2s.Scope{s2s.ScopeSystemAdmin, s2s.ScopeTenantAdmin}, "tenant_admin", "system_admin"))

	// Configurar módulos en orden de dependencias
	// 1. User Module (independiente) - retorna UserFinderService con account_app.
	//    ACC-E01 T7: parte del módulo identity (users y credenciales, PROP-013).
	var userFinderService service.UserFinderService
	if !disabled["identity"] {
		userFinderService = userConfig.SetupUserModule(tenantScopedGroup, appDB)
	}

	// 2. Tenant Management Module (tenant-scoped): lectura/escritura por ID
	//    GET/PUT/DELETE /tenants/:id se mueven al grupo tenant-scoped para que
	//    servicios como onboarding (tenant:admin) puedan gestionar sus propios
	//    tenants sin necesitar system:admin. List/Plan/Features quedan en adminGroup.
	if !disabled["tenancy"] {
		tenantConfig.SetupTenantScopedModule(tenantScopedGroup, appDB, metricsRecorder)
	}

	// 3. Tenant Admin Module (cross-tenant global): list, plans, features → system:admin
	var tenantFeaturesUC *tenantUC.GetTenantFeaturesUseCase
	if !disabled["tenancy"] {
		tenantFeaturesUC = tenantConfig.SetupTenantModule(adminGroup, appDB, metricsRecorder)
	}

	// 4. Auth Module (depende de User y Tenant)
	// User finder para la fase pre-auth del login (iam_login). No registra rutas;
	// sólo se inyecta en el LoginUseCase.
	// El adapter convierte tenant_vo.TenantFeatures → auth_vo.TenantFeatures (anti-corruption layer)
	// ACC-E01 T7: el bloque de login completo (user finder de login incluido) se
	// monta sólo con identity — con el módulo desmontado no queda wiring huérfano.
	if !disabled["identity"] {
		loginUserRepo := userRepo.NewPostgresUserRepository(loginDB)
		loginUserFinder := userUC.NewUserFinderUseCase(loginUserRepo)
		tenantService := adapter.NewTenantFeaturesAdapter(tenantFeaturesUC)
		// ACC-E03 T4: authConfig y signingVerifier ya se crearon arriba (antes
		// del gate de revocación); acá se reutilizan — una sola carga de clave
		// y un solo verificador dual en el proceso.
		config.SetupAuthModule(apiV1, appDB, loginDB, authRepoApp, userFinderService, loginUserFinder, tenantService, signingVerifier, authConfig)
	}

	// 5. Plan Module (independiente)
	if !disabled["plans"] {
		planConfig.SetupPlanModule(adminGroup, appDB)
	}

	// 6. Role Module (catálogo global, ACC-E02 T10). `roles` no lleva RLS ni
	//    tenant_id: el gate de scope es la única defensa de la tabla. Las rutas
	//    de lectura (GET /roles, GET /roles/:id) quedan en tenantScopedGroup
	//    (legibles por system:admin o tenant:admin); las de escritura
	//    (POST/PUT/DELETE) pasan a adminGroup (system:admin únicamente), cerrando
	//    la escalada por la que un tenant:admin podía crearse un rol SYSTEM_ADMIN
	//    y mutar los roles de sistema existentes.
	//    ACC-E01 T7: la única ruta propia del módulo access. El gate de scopes
	//    S2S/JWT (s2sRegistry + authFactory) NO es wiring de access: es defensa
	//    del HTTP compartido y protege las rutas de users/tenants/plans — se
	//    monta siempre.
	if !disabled["access"] {
		roleConfig.SetupRoleModule(tenantScopedGroup, adminGroup, appDB)
	}

	// 7. Tenant Provision Module — SOLO POST /tenants para whatsapp-agent/onboarding con scope tenant:provision.
	// También permitimos system:admin (es un super-scope) para no forzar a sales
	// a tener una key separada de tenant:provision mientras migran.
	if !disabled["tenancy"] {
		provisionGroup := apiV1.Group("", authFactory.RequireScopes([]s2s.Scope{s2s.ScopeTenantProvision, s2s.ScopeSystemAdmin}, "system_admin"))
		tenantConfig.SetupTenantProvisionModule(provisionGroup, appDB, metricsRecorder)
	}

	return router
}

// modulesDisabled parsea la variable de entorno MODULES_DISABLED y devuelve el
// conjunto de módulos que el wiring NO debe montar. Claves canónicas (módulos
// de PROP-013), con el wiring de cada una:
//
//	identity            → SetupTokenRevocationGate + SetupUserModule + SetupAuthModule
//	                      (alias histórico "auth": el wiring de login/refresh/logout/
//	                      revoke es SetupAuthModule — el smoke de ACC-E01 T7 lo usa)
//	access              → SetupRoleModule (catálogo de roles)
//	tenancy             → SetupTenantScopedModule + SetupTenantModule + SetupTenantProvisionModule
//	plans               → SetupPlanModule
//	subscription        → sin wiring hoy (módulo vacío hasta ACC-E06); clave aceptada
//	onboarding          → sin wiring hoy (módulo vacío hasta ACC-E05); clave aceptada
//
// subscription y onboarding se aceptan igual para que el smoke del mecanismo no
// dependa de que los módulos estén llenos: con MODULES_DISABLED=subscription el
// servicio arranca con el switch armado y el login intacto — la condición de
// diseño de PROP-013 (login ↮ subscription) queda re-verificada en runtime
// cuando ACC-E06 llene el módulo.
//
// Un módulo listado que no existe en la lista canónica se ignora en silencio:
// nombres desconocidos no tumban el arranque.
func modulesDisabled() map[string]bool {
	disabled := make(map[string]bool)
	for _, raw := range strings.Split(os.Getenv("MODULES_DISABLED"), ",") {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" {
			continue
		}
		if name == "auth" {
			// Alias histórico — el wiring de login se llama SetupAuthModule y
			// el criterio (b) de ACC-E01 T7 verifica el switch con esta clave.
			name = "identity"
		}
		disabled[name] = true
	}
	if len(disabled) > 0 {
		names := make([]string, 0, len(disabled))
		for name := range disabled {
			names = append(names, name)
		}
		log.Printf("MODULES_DISABLED activo — módulos no montados: %v", names)
	}
	return disabled
}
