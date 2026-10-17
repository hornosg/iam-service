package usecase

import (
	"context"

	"github.com/hornosg/iam-service/src/tenancy/application/response"
	"github.com/hornosg/iam-service/src/tenancy/domain/port"
	"github.com/hornosg/iam-service/src/tenancy/domain/value_object"
)

type GetTenantBySlugUseCase struct {
	tenantRepo port.TenantRepository
}

func NewGetTenantBySlugUseCase(tenantRepo port.TenantRepository) *GetTenantBySlugUseCase {
	return &GetTenantBySlugUseCase{
		tenantRepo: tenantRepo,
	}
}

func (uc *GetTenantBySlugUseCase) Execute(ctx context.Context, namespace, slug string) (*response.TenantResponse, error) {
	namespace, err := value_object.ResolveProductNamespace(namespace)
	if err != nil {
		return nil, err
	}
	tenant, err := uc.tenantRepo.GetBySlug(ctx, namespace, slug)
	if err != nil {
		return nil, err
	}

	return response.NewTenantResponse(tenant), nil
}
