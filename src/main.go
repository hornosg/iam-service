package main

import (
	"database/sql"
	"log"
	"os"
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	_ "github.com/lib/pq"
	tenantmw "github.com/mercadocercano/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"iam/src/auth/infrastructure/adapter"
	"iam/src/auth/infrastructure/config"
	planConfig "iam/src/plan/infrastructure/config"
	roleConfig "iam/src/role/infrastructure/config"
	tenantConfig "iam/src/tenant/infrastructure/config"
	userConfig "iam/src/user/infrastructure/config"

	sharedport "github.com/mercadocercano/go-shared/domain/port"
	sharedlog "github.com/mercadocercano/go-shared/infrastructure/logging"
	sharedmetrics "github.com/mercadocercano/go-shared/infrastructure/metrics"
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

	// Configuración del router
	router := gin.New() // Usar gin.New() para evitar middlewares duplicados

	// Agregar middlewares básicos necesarios
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// Validación de tenant (X-Tenant-ID vs JWT tenant_id)
	securityLogger := sharedlog.NewSecurityLogger("iam")
	router.Use(tenantmw.TenantValidation(tenantmw.TenantValidationConfig{
		JWTSecret: os.Getenv("JWT_SECRET"),
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

	// Configurar módulos en orden de dependencias
	// 1. User Module (independiente) - retorna UserFinderService
	userFinderService := userConfig.SetupUserModule(apiV1, db)

	// 2. Tenant Module (independiente) - retorna GetTenantFeaturesUseCase
	tenantFeaturesUC := tenantConfig.SetupTenantModule(apiV1, db, metricsRecorder)

	// 3. Auth Module (depende de User y Tenant)
	// El adapter convierte tenant_vo.TenantFeatures → auth_vo.TenantFeatures (anti-corruption layer)
	tenantService := adapter.NewTenantFeaturesAdapter(tenantFeaturesUC)
	authConfig := config.NewAuthModuleConfigFromEnv()
	config.SetupAuthModule(apiV1, db, userFinderService, tenantService, authConfig)

	// 4. Plan Module (independiente)
	planConfig.SetupPlanModule(apiV1, db)

	// 5. Role Module (independiente)
	roleConfig.SetupRoleModule(apiV1, db)

	// Iniciar el servidor
	port := getEnv("PORT", "8080")
	log.Printf("Starting IAM server on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}

func setupDatabase() (*sql.DB, error) {
	// Configuración de la base de datos desde variables de entorno
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "postgres")
	password := getEnv("DB_PASSWORD", "postgres")
	dbname := getEnv("DB_NAME", "iam_db")
	sslmode := getEnv("DB_SSLMODE", "disable")

	dsn := "host=" + host + " port=" + port + " user=" + user + " password=" + password + " dbname=" + dbname + " sslmode=" + sslmode

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	// Verificar la conexión
	if err := db.Ping(); err != nil {
		return nil, err
	}

	log.Println("Successfully connected to database")
	return db, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
