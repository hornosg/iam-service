package usecase

import (
	"context"

	"iam/src/tenant/application/response"
	"iam/src/tenant/domain/port"
)

type GetTenantBySlugUseCase struct {
	tenantRepo port.TenantRepository
}

func NewGetTenantBySlugUseCase(tenantRepo port.TenantRepository) *GetTenantBySlugUseCase {
	return &GetTenantBySlugUseCase{
		tenantRepo: tenantRepo,
	}
}

func (uc *GetTenantBySlugUseCase) Execute(ctx context.Context, slug string) (*response.TenantResponse, error) {
	tenant, err := uc.tenantRepo.GetBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}

	return response.NewTenantResponse(tenant), nil
}
