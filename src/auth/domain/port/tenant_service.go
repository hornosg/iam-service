package port

import (
	"context"

	"github.com/google/uuid"

	"iam/src/auth/domain/value_object"
)

// TenantService is the port auth uses to fetch tenant feature flags for JWT generation.
// It returns auth's own TenantFeatures type — not the tenant module's type.
// The conversion is handled by the infrastructure adapter (auth/infrastructure/adapter).
type TenantService interface {
	Execute(ctx context.Context, tenantID uuid.UUID) (*value_object.TenantFeatures, error)
}
