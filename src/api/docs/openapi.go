package docs

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// OpenAPIHandler maneja la documentación OpenAPI
type OpenAPIHandler struct {
	specPath string
}

// NewOpenAPIHandler crea una nueva instancia del handler de OpenAPI
func NewOpenAPIHandler(specPath string) *OpenAPIHandler {
	return &OpenAPIHandler{
		specPath: specPath,
	}
}

// RegisterRoutes registra las rutas para la documentación OpenAPI
func (h *OpenAPIHandler) RegisterRoutes(router *gin.Engine) {
	// Ruta para la documentación OpenAPI
	router.GET("/api-docs", func(c *gin.Context) {
		c.HTML(http.StatusOK, "swagger.html", gin.H{
			"title": "IAM API Documentation",
			"url":   "http://localhost:8080/openapi.yaml",
		})
	})

	// Servir el archivo de especificación OpenAPI
	if h.specPath != "" {
		router.StaticFile("/openapi.yaml", h.specPath)
	}
}
