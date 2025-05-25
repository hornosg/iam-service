package port

import (
	"context"
	tenant_vo "iam/src/tenant/domain/value_object"

	"github.com/google/uuid"
)

// TenantService define la interfaz para obtener información del tenant
type TenantService interface {
	Execute(ctx context.Context, tenantID uuid.UUID) (*tenant_vo.TenantFeatures, error)
}
