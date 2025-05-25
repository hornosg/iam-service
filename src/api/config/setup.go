package config

import (
	"database/sql"
	"iam/src/api/docs"
	"iam/src/api/health"

	"github.com/gin-gonic/gin"
)

// APIConfig contiene la configuración para el módulo API
type APIConfig struct {
	Version     string
	DB          *sql.DB
	OpenAPIPath string
}

// DefaultAPIConfig devuelve una configuración por defecto
func DefaultAPIConfig() APIConfig {
	return APIConfig{
		Version:     "1.0.0",
		OpenAPIPath: "./api-docs/openapi.yaml",
	}
}

// SetupAPIModule configura las rutas y handlers del módulo API
func SetupAPIModule(router *gin.Engine, apiGroup *gin.RouterGroup, config APIConfig) {
	// Configurar el healthcheck
	healthHandler := health.NewHandler(config.DB, config.Version)
	healthHandler.RegisterRoutes(apiGroup)

	// Configurar la documentación OpenAPI
	if config.OpenAPIPath != "" {
		openAPIHandler := docs.NewOpenAPIHandler(config.OpenAPIPath)
		openAPIHandler.RegisterRoutes(router)
	}
}
