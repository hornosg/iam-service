package usecase

import (
	"context"

	"iam/src/tenant/application/response"
	"iam/src/tenant/domain/port"

	"github.com/google/uuid"
)

type GetTenantByIDUseCase struct {
	tenantRepo port.TenantRepository
}

func NewGetTenantByIDUseCase(tenantRepo port.TenantRepository) *GetTenantByIDUseCase {
	return &GetTenantByIDUseCase{
		tenantRepo: tenantRepo,
	}
}

func (uc *GetTenantByIDUseCase) Execute(ctx context.Context, id uuid.UUID) (*response.TenantResponse, error) {
	tenant, err := uc.tenantRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return response.NewTenantResponse(tenant), nil
}
