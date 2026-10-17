package controller

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	sharedctx "github.com/hornosg/iam-service/src/shared/context"
	"github.com/hornosg/iam-service/src/tenancy/application/request"
	"github.com/hornosg/iam-service/src/tenancy/application/response"
	"github.com/hornosg/iam-service/src/tenancy/application/usecase"
	"github.com/hornosg/iam-service/src/tenancy/domain/value_object"
	"github.com/hornosg/iam-service/src/tenancy/infrastructure/audit"
)

// TenantConfigController maneja las peticiones HTTP para configuraciones.
// Mudado desde tenant-service (ACC-E04 T3). T4 lo monta detrás del gate de DD-1 (ADR-005):
// read/bootstrap S2S con el tenant de X-Tenant-ID leído tras el gate; set humano con el
// tenant del claim verificado.
type TenantConfigController struct {
	getConfigQuery   *usecase.GetTenantConfigUseCase
	setConfigCommand *usecase.SetTenantConfigUseCase
	bootstrapCommand *usecase.BootstrapTenantConfigUseCase
}

// NewTenantConfigController crea una nueva instancia del controlador
func NewTenantConfigController(
	getConfigQuery *usecase.GetTenantConfigUseCase,
	setConfigCommand *usecase.SetTenantConfigUseCase,
	bootstrapCommand *usecase.BootstrapTenantConfigUseCase,
) *TenantConfigController {
	return &TenantConfigController{
		getConfigQuery:   getConfigQuery,
		setConfigCommand: setConfigCommand,
		bootstrapCommand: bootstrapCommand,
	}
}

// RegisterReadRoute monta GET /tenant/config/:key con la cadena dada (ACC-E04 T4,
// ADR-005 DD-1): auditoría → gate Authorize(config:read) → handler. El orden ES el
// control: el handler no lee X-Tenant-ID si el gate no pasó.
func (c *TenantConfigController) RegisterReadRoute(router *gin.RouterGroup, chain ...gin.HandlerFunc) {
	router.GET("/tenant/config/:key", append(chain, c.GetConfig)...)
}

// RegisterSetRoute monta POST /tenant/config detrás del gate humano (tenant_admin):
// el tenant sale del claim verificado por Authorize, nunca de X-Tenant-ID.
func (c *TenantConfigController) RegisterSetRoute(router *gin.RouterGroup, chain ...gin.HandlerFunc) {
	router.POST("/tenant/config", append(chain, c.SetConfig)...)
}

// RegisterBootstrapRoute monta POST /tenant/bootstrap con la cadena dada:
// auditoría → gate Authorize(tenant:provision) → handler.
func (c *TenantConfigController) RegisterBootstrapRoute(router *gin.RouterGroup, chain ...gin.HandlerFunc) {
	router.POST("/tenant/bootstrap", append(chain, c.BootstrapConfig)...)
}

// tenantFromGatedHeader lee X-Tenant-ID para las rutas S2S. Sólo se invoca desde
// el handler, que corre únicamente si Authorize dejó pasar el request (paso 2 de
// la secuencia de DD-1). Rechaza ausente, no-UUID y uuid.Nil con 400; con un UUID
// válido lo publica para la auditoría y lo propaga al context de la request.
func tenantFromGatedHeader(ctx *gin.Context) (uuid.UUID, bool) {
	tenantID, err := uuid.Parse(ctx.GetHeader("X-Tenant-ID"))
	if err != nil || tenantID == uuid.Nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "X-Tenant-ID header must be a valid non-nil UUID"})
		return uuid.Nil, false
	}
	audit.SetTenant(ctx, tenantID)
	ctx.Request = ctx.Request.WithContext(sharedctx.WithTenantID(ctx.Request.Context(), tenantID))
	return tenantID, true
}

// readNamespaces son los namespaces que el scope config:read puede leer (mínimo privilegio).
var readNamespaces = map[string]bool{"catalog": true}

func readableNamespace(key string) bool {
	ck, err := value_object.NewConfigKey(key)
	return err == nil && readNamespaces[ck.Namespace()]
}

// GetConfig obtiene una configuración por clave
// @Summary Get tenant configuration by key
// @Description Obtiene el valor de una configuración específica del tenant
// @Tags tenant-config
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param key path string true "Configuration Key (e.g., catalog.stock_policy)"
// @Success 200 {object} response.SimpleConfigResponse
// @Success 404 {object} response.SimpleConfigResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/tenant/config/{key} [get]
func (c *TenantConfigController) GetConfig(ctx *gin.Context) {
	// ADR-005 DD-1 (b): el tenant pedido se lee SÓLO acá, tras el gate config:read.
	// El aislamiento lo ejerce RLS en la TX del repo (app.tenant_id = este UUID).
	tenantID, ok := tenantFromGatedHeader(ctx)
	if !ok {
		return
	}

	// Obtener key del path
	key := ctx.Param("key")
	if key == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "Configuration key is required",
		})
		return
	}

	// ACC-E04 T4 M1: el scope S2S config:read cubre sólo el namespace catalog.*. Una key de
	// otro namespace se rechaza antes de tocar la DB; sale como denied en la auditoría.
	if !readableNamespace(key) {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "Configuration namespace not readable with this scope"})
		return
	}

	// Ejecutar query
	config, exists, err := c.getConfigQuery.Execute(ctx.Request.Context(), tenantID, key)
	if err != nil {
		log.Printf("Error getting config for tenant %s: %v", tenantID, err)
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "Internal server error",
		})
		return
	}

	// Si no existe, devolver 404 con value null
	if !exists {
		ctx.JSON(http.StatusNotFound, response.NewSimpleConfigResponse(key, nil))
		return
	}

	// Devolver configuración encontrada
	ctx.JSON(http.StatusOK, response.NewSimpleConfigResponse(key, &config.Value))
}

// SetConfig establece o actualiza una configuración
// @Summary Set tenant configuration
// @Description Crea o actualiza una configuración del tenant (upsert)
// @Tags tenant-config
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param request body request.SetTenantConfigRequest true "Configuration data"
// @Success 200 {object} response.TenantConfigResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/tenant/config [post]
func (c *TenantConfigController) SetConfig(ctx *gin.Context) {
	// tenant_id SIEMPRE del claim JWT que verificó Authorize (rol tenant_admin, ADR-005
	// §Camino de escritura) — nunca del header X-Tenant-ID, que se ignora. Fail-closed: 401
	// si el claim no está en el contexto (la RLS sin app.tenant_id rechaza igual). Config L4.
	tenantIDStr := ctx.GetString("tenant_id")
	if tenantIDStr == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"error": "tenant_id missing from request context",
		})
		return
	}

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid tenant ID format",
		})
		return
	}

	// Parsear body
	var req request.SetTenantConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request body: " + err.Error(),
		})
		return
	}

	// Validar request
	if err := req.Validate(); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Ejecutar command
	config, err := c.setConfigCommand.Execute(ctx.Request.Context(), tenantID, req.Key, req.Value)
	if err != nil {
		log.Printf("Error setting config for tenant %s, key %s: %v", tenantID, req.Key, err)
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "Internal server error",
		})
		return
	}

	// Devolver configuración guardada
	ctx.JSON(http.StatusOK, response.TenantConfigFromEntity(config))
}

// BootstrapConfig inicializa la configuración default de un tenant
// @Summary Bootstrap tenant configuration
// @Description Crea configuraciones por defecto para un tenant nuevo (idempotente)
// @Tags tenant-config
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/tenant/bootstrap [post]
func (c *TenantConfigController) BootstrapConfig(ctx *gin.Context) {
	// ADR-005 DD-1: bootstrap S2S (tenant:provision); el tenant se lee sólo tras el gate.
	tenantID, ok := tenantFromGatedHeader(ctx)
	if !ok {
		return
	}

	createdCount, err := c.bootstrapCommand.Execute(ctx.Request.Context(), tenantID)
	if err != nil {
		log.Printf("Bootstrap de config falló para tenant %s: %v", tenantID, err)
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "Internal server error",
		})
		return
	}

	// Siempre responder 200 (idempotente)
	ctx.JSON(http.StatusOK, gin.H{
		"success":         true,
		"message":         "Tenant configuration bootstrapped successfully",
		"tenant_id":       tenantID.String(),
		"configs_created": createdCount,
	})
}
