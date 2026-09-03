package context

import (
	"context"

	"github.com/google/uuid"
)

type tenantIDKey struct{}

// WithTenantID devuelve un contexto que porta el tenant_id para que los
// repositorios/adapters que corren bajo account_app fijen el GUC RLS
// (app.tenant_id) de forma fail-closed.
func WithTenantID(ctx context.Context, tenantID uuid.UUID) context.Context {
	return context.WithValue(ctx, tenantIDKey{}, tenantID)
}

// TenantIDFromContext extrae el tenant_id del contexto. El valor puede venir
// como uuid.UUID (casos de uso) o como string (middlewares Gin que deserializan
// claims JWT).
func TenantIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	v := ctx.Value(tenantIDKey{})
	if v == nil {
		return uuid.UUID{}, false
	}
	switch t := v.(type) {
	case uuid.UUID:
		return t, true
	case string:
		id, err := uuid.Parse(t)
		if err != nil {
			return uuid.UUID{}, false
		}
		return id, true
	default:
		return uuid.UUID{}, false
	}
}

type systemAdminKey struct{}

// WithSystemAdmin marca en el contexto que el gate de autorización YA verificó
// la autoridad system_admin del llamador (scope S2S `system:admin` resuelto
// contra el registry, o rol `system_admin` verificado contra allowedRoles).
// ACC-E02 T8d: este flag alimenta el GUC `app.is_system_admin`, que abre el
// branch cross-tenant de la policy de `tenants` (019) — la tabla entera. Por eso
// su ÚNICO origen válido es la decisión de autorización de Authorize: jamás se
// fija desde un claim crudo del JWT (un `is_system_admin` inyectado en un token
// con rol tenant_admin no abre nada — test de T8d).
func WithSystemAdmin(ctx context.Context) context.Context {
	return context.WithValue(ctx, systemAdminKey{}, true)
}

// IsSystemAdminFromContext reporta si el gate de autorización verificó la
// autoridad system_admin para esta request. Ausencia → false: los consumidores
// del flag deben tratarlo fail-closed.
func IsSystemAdminFromContext(ctx context.Context) bool {
	v, ok := ctx.Value(systemAdminKey{}).(bool)
	return ok && v
}
