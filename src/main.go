package main

import (
	"database/sql"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"

	"iam/src/auth/infrastructure/config"
	planConfig "iam/src/plan/infrastructure/config"
	roleConfig "iam/src/role/infrastructure/config"
	tenantConfig "iam/src/tenant/infrastructure/config"
	userConfig "iam/src/user/infrastructure/config"
)

func main() {
	// Configuración de la base de datos
	db, err := setupDatabase()
	if err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}
	defer db.Close()

	// Configuración del router
	router := gin.Default()

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

	// Configurar módulos en orden de dependencias
	// 1. User Module (independiente) - retorna UserFinderService
	userFinderService := userConfig.SetupUserModule(apiV1, db)

	// 2. Tenant Module (independiente) - retorna TenantService
	tenantService := tenantConfig.SetupTenantModule(apiV1, db)

	// 3. Auth Module (depende de User y Tenant)
	authConfig := config.DefaultAuthModuleConfig()
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
