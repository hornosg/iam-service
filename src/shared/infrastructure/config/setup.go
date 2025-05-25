package config

import (
	"iam/src/shared/infrastructure/middleware"

	"github.com/gin-gonic/gin"
)

// SharedConfig contiene la configuración para el módulo compartido
type SharedConfig struct {
	EnableGzip            bool
	AlwaysTryDecompress   bool
	ForceGzipCompression  bool
	ForceGzipCheckSupport bool     // Verifica si el cliente soporta gzip antes de forzar compresión
	ForceGzipPaths        []string // Rutas donde forzar compresión
	GzipExcludedPaths     []string
}

// DefaultSharedConfig devuelve una configuración por defecto
func DefaultSharedConfig() SharedConfig {
	return SharedConfig{
		EnableGzip:            true,
		AlwaysTryDecompress:   true,
		ForceGzipCompression:  false,
		ForceGzipCheckSupport: true,
		ForceGzipPaths:        []string{"/iam/api/v1/users"},
		GzipExcludedPaths:     []string{"/health", "/metrics", "/api-docs", "/iam/api/v1/auth"},
	}
}

// SetupSharedMiddleware configura los middlewares compartidos
func SetupSharedMiddleware(router *gin.Engine, config SharedConfig) {
	// Aplicar middleware para intentar descomprimir todas las solicitudes entrantes si está habilitado
	if config.AlwaysTryDecompress {
		router.Use(middleware.GzipReader())
	}

	// Aplicar middleware de compresión gzip si está habilitado
	if config.EnableGzip {
		gzipOpts := middleware.GzipOptions{
			ExcludedPaths: config.GzipExcludedPaths,
		}
		router.Use(middleware.GzipMiddleware(gzipOpts))

		// Configurar rutas que siempre deben usar compresión gzip
		if config.ForceGzipCompression && len(config.ForceGzipPaths) > 0 {
			forceGzipOpts := middleware.ForceGzipOptions{
				CheckClientSupport: config.ForceGzipCheckSupport,
			}

			// Ejemplo de cómo aplicar compresión forzada a rutas específicas
			for _, path := range config.ForceGzipPaths {
				router.Group(path).Use(middleware.ForceGzipMiddleware(forceGzipOpts))
			}
		}
	}

	// Aquí se pueden agregar más middlewares compartidos en el futuro
	// Por ejemplo:
	// - Logging
	// - CORS
	// - Medición de rendimiento
	// - Autenticación/Autorización
}
