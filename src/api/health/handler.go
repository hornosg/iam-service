package health

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthStatus representa la respuesta del endpoint de health
type HealthStatus struct {
	Status    string `json:"status"`
	Database  string `json:"database,omitempty"`
	Version   string `json:"version,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

// Handler maneja el endpoint de health check
type Handler struct {
	db      *sql.DB
	version string
}

// NewHandler crea una nueva instancia del handler de health
func NewHandler(db *sql.DB, version string) *Handler {
	return &Handler{
		db:      db,
		version: version,
	}
}

// RegisterRoutes registra las rutas de health check
func (h *Handler) RegisterRoutes(router *gin.RouterGroup) {
	router.GET("/health", h.HealthCheck)
}

// HealthCheck verifica el estado del sistema
func (h *Handler) HealthCheck(c *gin.Context) {
	status := "ok"
	dbStatus := "ok"

	// Verificar la conexión a la base de datos si está configurada
	if h.db != nil {
		if err := h.db.Ping(); err != nil {
			status = "degraded"
			dbStatus = "error"
		}
	}

	response := HealthStatus{
		Status:   status,
		Database: dbStatus,
		Version:  h.version,
	}

	c.JSON(http.StatusOK, response)
}
