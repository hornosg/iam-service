package usecase

import (
	"context"

	"iam/src/tenant/application/response"
	"iam/src/tenant/domain/port"
	"iam/src/tenant/domain/value_object"

	"github.com/google/uuid"
)

type UpdateTenantFeaturesRequest struct {
	TenantID         uuid.UUID `json:"tenant_id" binding:"required"`
	FriendsFamily    bool      `json:"friends_family"`
	PremiumAnalytics bool      `json:"premium_analytics"`
}

type UpdateTenantFeaturesUseCase struct {
	tenantRepo port.TenantRepository
}

func NewUpdateTenantFeaturesUseCase(tenantRepo port.TenantRepository) *UpdateTenantFeaturesUseCase {
	return &UpdateTenantFeaturesUseCase{
		tenantRepo: tenantRepo,
	}
}

func (uc *UpdateTenantFeaturesUseCase) Execute(ctx context.Context, req *UpdateTenantFeaturesRequest) (*response.TenantResponse, error) {
	// Obtener el tenant
	tenant, err := uc.tenantRepo.GetByID(ctx, req.TenantID)
	if err != nil {
		return nil, err
	}

	// Crear nuevos features
	newFeatures := value_object.NewTenantFeaturesWithValues(req.FriendsFamily, req.PremiumAnalytics)

	// Actualizar features del tenant
	tenant.UpdateFeatures(newFeatures)

	// Guardar cambios
	if err := uc.tenantRepo.Update(ctx, tenant); err != nil {
		return nil, err
	}

	return response.NewTenantResponse(tenant), nil
}
