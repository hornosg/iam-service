package monitoring

import (
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// HTTP metrics
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "endpoint", "status", "tenant_id"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint", "tenant_id"},
	)

	// Database metrics
	databaseConnectionsActive = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "database_connections_active",
			Help: "Number of active database connections",
		},
	)

	databaseQueriesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "database_queries_total",
			Help: "Total number of database queries",
		},
		[]string{"operation", "table"},
	)

	// Business metrics
	userAuthenticationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "user_authentications_total",
			Help: "Total number of user authentication attempts",
		},
		[]string{"tenant_id", "status"},
	)

	activeUsersGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "active_users",
			Help: "Number of currently active users",
		},
		[]string{"tenant_id"},
	)

	tenantsCreatedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tenants_created_total",
			Help: "Total number of tenants created",
		},
		[]string{"plan_id", "status"},
	)
)

func init() {
	// Register metrics
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(databaseConnectionsActive)
	prometheus.MustRegister(databaseQueriesTotal)
	prometheus.MustRegister(userAuthenticationsTotal)
	prometheus.MustRegister(activeUsersGauge)
	prometheus.MustRegister(tenantsCreatedTotal)
}

// PrometheusMiddleware middleware para capturar métricas HTTP
func PrometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Obtener tenant_id del header o contexto
		tenantID := c.GetHeader("X-Tenant-ID")
		if tenantID == "" {
			tenantID = "unknown"
		}

		c.Next()

		duration := time.Since(start)
		status := strconv.Itoa(c.Writer.Status())

		httpRequestsTotal.WithLabelValues(
			c.Request.Method,
			c.FullPath(),
			status,
			tenantID,
		).Inc()

		httpRequestDuration.WithLabelValues(
			c.Request.Method,
			c.FullPath(),
			tenantID,
		).Observe(duration.Seconds())
	}
}

// RecordDatabaseQuery registra métricas de consultas a la base de datos
func RecordDatabaseQuery(operation, table string) {
	databaseQueriesTotal.WithLabelValues(operation, table).Inc()
}

// SetDatabaseConnections actualiza el número de conexiones activas
func SetDatabaseConnections(count float64) {
	databaseConnectionsActive.Set(count)
}

// RecordUserAuthentication registra intentos de autenticación
func RecordUserAuthentication(tenantID, status string) {
	userAuthenticationsTotal.WithLabelValues(tenantID, status).Inc()
}

// SetActiveUsers actualiza el número de usuarios activos
func SetActiveUsers(tenantID string, count float64) {
	activeUsersGauge.WithLabelValues(tenantID).Set(count)
}

// RecordTenantCreated registra la creación de un nuevo tenant
func RecordTenantCreated(planID, status string) {
	tenantsCreatedTotal.WithLabelValues(planID, status).Inc()
}

// StartPrometheusServer inicia el servidor de métricas si está habilitado
func StartPrometheusServer() {
	enabled := os.Getenv("PROMETHEUS_ENABLED")
	if enabled != "true" {
		return
	}

	port := os.Getenv("PROMETHEUS_PORT")
	if port == "" {
		port = "2112"
	}

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			panic("Failed to start Prometheus metrics server: " + err.Error())
		}
	}()
}
