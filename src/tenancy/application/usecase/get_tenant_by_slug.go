package usecase

import (
	"context"

	"github.com/hornosg/iam-service/src/tenancy/application/response"
	"github.com/hornosg/iam-service/src/tenancy/domain/port"
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
