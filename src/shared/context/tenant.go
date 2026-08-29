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
