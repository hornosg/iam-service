package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/hornosg/go-shared/infrastructure/env"
	tenantmw "github.com/hornosg/go-shared/infrastructure/middleware"
	"github.com/hornosg/go-shared/infrastructure/postgres"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"iam/src/auth/infrastructure/adapter"
	"iam/src/auth/infrastructure/config"
	authmw "iam/src/auth/infrastructure/middleware"
	planConfig "iam/src/plan/infrastructure/config"
	roleConfig "iam/src/role/infrastructure/config"
	tenantConfig "iam/src/tenant/infrastructure/config"
	userConfig "iam/src/user/infrastructure/config"

	sharedport "github.com/hornosg/go-shared/domain/port"
	sharedlog "github.com/hornosg/go-shared/infrastructure/logging"
	sharedmetrics "github.com/hornosg/go-shared/infrastructure/metrics"
	sharedmigrate "github.com/hornosg/go-shared/migrate"

	iamroot "iam"
)

var slugRegexp = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]*$`)

func init() {
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		v.RegisterValidation("slug", func(fl validator.FieldLevel) bool {
			return slugRegexp.MatchString(fl.Field().String())
		})
	}
}

func main() {
	// Configuración de la base de datos
	db, err := setupDatabase()
	if err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}
	defer db.Close()

	// Migraciones versionadas in-app (ADR-001) — fail-fast antes de servir tráfico.
	dbName := env.Get("DB_NAME", "iam_db")
	if err := sharedmigrate.RunMigrations(db, iamroot.MigrationsFS, dbName); err != nil {
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
	// autorizan por X-API-Key; humanos por JWT + rol.
	//   - adminGroup       (cross-tenant global): tenants, plans → system_admin
	//   - tenantScopedGroup (tenant-scoped):      users, roles   → tenant_admin/system_admin
	s2sAPIKey := env.Get("S2S_API_KEY", "")
	jwtSecret := os.Getenv("JWT_SECRET")
	adminGroup := apiV1.Group("", authmw.Authorize(jwtSecret, serviceNamespace, s2sAPIKey, "system_admin"))
	tenantScopedGroup := apiV1.Group("", authmw.Authorize(jwtSecret, serviceNamespace, s2sAPIKey, "tenant_admin", "system_admin"))

	// Configurar módulos en orden de dependencias
	// 1. User Module (independiente) - retorna UserFinderService
	userFinderService := userConfig.SetupUserModule(tenantScopedGroup, db)

	// 2. Tenant Module (independiente) - retorna GetTenantFeaturesUseCase
	tenantFeaturesUC := tenantConfig.SetupTenantModule(adminGroup, db, metricsRecorder)

	// 3. Auth Module (depende de User y Tenant)
	// El adapter convierte tenant_vo.TenantFeatures → auth_vo.TenantFeatures (anti-corruption layer)
	tenantService := adapter.NewTenantFeaturesAdapter(tenantFeaturesUC)
	authConfig := config.NewAuthModuleConfigFromEnv()
	config.SetupAuthModule(apiV1, db, userFinderService, tenantService, authConfig)

	// 4. Plan Module (independiente)
	planConfig.SetupPlanModule(adminGroup, db)

	// 5. Role Module (independiente)
	roleConfig.SetupRoleModule(tenantScopedGroup, db)

	// Iniciar el servidor
	port := env.Get("PORT", "8080")
	log.Printf("Starting IAM server on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}

func setupDatabase() (*sql.DB, error) {
	// Configuración de la base de datos desde variables de entorno
	host := env.Get("DB_HOST", "localhost")
	port := env.Get("DB_PORT", "5432")
	user := env.Get("DB_USER", "postgres")
	password := env.Get("DB_PASSWORD", "postgres")
	dbname := env.Get("DB_NAME", "iam_db")
	sslmode := env.Get("DB_SSLMODE", "disable")

	db, err := postgres.Connect(postgres.Config{
		Host:     host,
		Port:     port,
		User:     user,
		Password: password,
		DBName:   dbname,
		SSLMode:  sslmode,
	})
	if err != nil {
		return nil, err
	}

	postgres.StartPoolMonitor(context.Background(), db, postgres.MonitorOptions{
		Service: "iam-service",
		DBName:  dbname,
	})

	log.Println("Successfully connected to database")
	return db, nil
}
