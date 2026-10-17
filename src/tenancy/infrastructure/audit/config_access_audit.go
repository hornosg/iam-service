// Package audit implementa la auditoría cross-tenant de las rutas S2S de config
// (ACC-E04 T4, ADR-005 §Auditoría cross-tenant). Vive en Infrastructure: Gin y el
// logger canónico no bajan a Application/Domain.
package audit

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Event es el nombre canónico del evento de acceso a config.
const Event = "tenant.config_access"

// Operaciones auditadas: constantes de ruta, nunca derivadas del request.
const (
	OperationRead      = "config.read"
	OperationBootstrap = "config.bootstrap"
)

// Outcomes cerrados (enum según el status HTTP final, nunca texto de error).
const (
	OutcomeSuccess        = "success"
	OutcomeNotFound       = "not_found"
	OutcomeDenied         = "denied"
	OutcomeInvalidRequest = "invalid_request"
	OutcomeError          = "error"
)

// Unattributed es la identidad cuando el gate no publicó un servicio verificado
// (sin key, key desconocida, scope insuficiente). Nunca se adivina el servicio.
const Unattributed = "unattributed"

// Claves de gin.Context que el wrapper publica/consume.
const (
	// TenantKey la fija el handler con el UUID ya validado (no nulo) que usa RLS.
	TenantKey = "config_audit_tenant_id"
	// CorrelationKey la fija el wrapper con un UUID generado localmente.
	CorrelationKey = "correlation_id"
)

// Emitter es el puerto del adapter canónico. En producción lo satisface
// go-shared/infrastructure/logging.CanonicalLogger (Emit retorna void: no hay
// acuse de escritura ni durabilidad, ADR-005 §Fallo del sink y límites).
type Emitter interface {
	Emit(level, event string, fields map[string]any)
}

// SetTenant publica el tenant validado para la auditoría. Sólo lo llama el handler
// tras parsear el UUID y rechazar uuid.Nil.
func SetTenant(c *gin.Context, tenantID uuid.UUID) {
	if tenantID != uuid.Nil {
		c.Set(TenantKey, tenantID)
	}
}

// ConfigAccess devuelve el wrapper de auditoría de una ruta S2S de config. Se
// monta ANTES del gate Authorize para observar también 401/403. No autoriza ni
// hace lookup de keys: tras c.Next() lee el resultado final y la identidad que
// SÓLO Authorize publica (`s2s_service` con `s2s=true`). Headers de identidad del
// llamador (X-Service-Name, s2s_service, ...) no tienen efecto.
//
// Fail-open sólo de auditoría: Emit no devuelve error y el wrapper no cambia la
// respuesta ni la decisión de auth/RLS. El emitter es obligatorio: un nil es un
// error de composición y falla en el arranque, no en silencio por request.
func ConfigAccess(emitter Emitter, operation string) gin.HandlerFunc {
	if emitter == nil {
		panic("audit.ConfigAccess: emitter canónico obligatorio (ADR-005 §Auditoría)")
	}
	if operation != OperationRead && operation != OperationBootstrap {
		panic(fmt.Sprintf("audit.ConfigAccess: operación no auditada: %q", operation))
	}
	return func(c *gin.Context) {
		correlationID := uuid.NewString()
		c.Set(CorrelationKey, correlationID)

		panicked := true
		defer func() {
			status := c.Writer.Status()
			if panicked {
				// El handler entró en pánico: Recovery escribirá 500 después.
				status = http.StatusInternalServerError
			}
			emit(emitter, c, operation, correlationID, status)
		}()

		c.Next()
		panicked = false
	}
}

func emit(emitter Emitter, c *gin.Context, operation, correlationID string, status int) {
	outcome, level := classify(status)
	fields := map[string]any{
		"s2s_service":    identity(c),
		"operation":      operation,
		"outcome":        outcome,
		"correlation_id": correlationID,
	}
	if v, ok := c.Get(TenantKey); ok {
		if tid, ok := v.(uuid.UUID); ok && tid != uuid.Nil {
			fields["tenant_id"] = tid.String()
		}
	}
	emitter.Emit(level, Event, fields)
}

// identity devuelve el servicio que publicó Authorize al pasar el gate; si no
// hay identidad verificada, el literal unattributed.
func identity(c *gin.Context) string {
	if !c.GetBool("s2s") {
		return Unattributed
	}
	if svc := c.GetString("s2s_service"); svc != "" {
		return svc
	}
	return Unattributed
}

// classify mapea el status HTTP final a outcome y nivel: éxitos info,
// denegaciones/invalid warn, fallos error.
func classify(status int) (outcome, level string) {
	switch {
	case status >= 500:
		return OutcomeError, "error"
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return OutcomeDenied, "warn"
	case status == http.StatusNotFound:
		return OutcomeNotFound, "info"
	case status >= 400:
		return OutcomeInvalidRequest, "warn"
	case status >= 200 && status < 300:
		return OutcomeSuccess, "info"
	default:
		return OutcomeError, "error"
	}
}
