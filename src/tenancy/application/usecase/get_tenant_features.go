package usecase

import (
	"context"
	"github.com/hornosg/iam-service/src/tenancy/domain/port"
	"github.com/hornosg/iam-service/src/tenancy/domain/value_object"

	"github.com/google/uuid"
)

type GetTenantFeaturesUseCase struct {
	tenantRepo port.TenantRepository
}

func NewGetTenantFeaturesUseCase(tenantRepo port.TenantRepository) *GetTenantFeaturesUseCase {
	return &GetTenantFeaturesUseCase{
		tenantRepo: tenantRepo,
	}
}

func (uc *GetTenantFeaturesUseCase) Execute(ctx context.Context, tenantID uuid.UUID) (*value_object.TenantFeatures, error) {
	tenant, err := uc.tenantRepo.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	return tenant.GetFeatures(), nil
}
