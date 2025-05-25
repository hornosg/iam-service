package usecase

import (
	"context"

	"iam/src/tenant/application/response"
	"iam/src/tenant/domain/port"
	"iam/src/tenant/domain/value_object"

	"github.com/google/uuid"
)

type ListTenantsUseCase struct {
	tenantRepo port.TenantRepository
}

func NewListTenantsUseCase(tenantRepo port.TenantRepository) *ListTenantsUseCase {
	return &ListTenantsUseCase{
		tenantRepo: tenantRepo,
	}
}

func (uc *ListTenantsUseCase) Execute(ctx context.Context, page, pageSize int) (*response.TenantListResponse, error) {
	// Calcular offset
	offset := (page - 1) * pageSize

	// Obtener tenants
	tenants, err := uc.tenantRepo.List(ctx, pageSize, offset)
	if err != nil {
		return nil, err
	}

	// Obtener total
	totalCount, err := uc.tenantRepo.Count(ctx)
	if err != nil {
		return nil, err
	}

	return response.NewTenantListResponse(tenants, totalCount, page, pageSize), nil
}

func (uc *ListTenantsUseCase) GetByOwner(ctx context.Context, ownerID uuid.UUID) (*response.TenantListResponse, error) {
	tenants, err := uc.tenantRepo.GetByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}

	return response.NewTenantListResponse(tenants, len(tenants), 1, len(tenants)), nil
}

func (uc *ListTenantsUseCase) GetByStatus(ctx context.Context, status value_object.TenantStatus, page, pageSize int) (*response.TenantListResponse, error) {
	// Calcular offset
	offset := (page - 1) * pageSize

	// Obtener tenants por status
	tenants, err := uc.tenantRepo.GetByStatus(ctx, status, pageSize, offset)
	if err != nil {
		return nil, err
	}

	// Obtener total por status
	totalCount, err := uc.tenantRepo.CountByStatus(ctx, status)
	if err != nil {
		return nil, err
	}

	return response.NewTenantListResponse(tenants, totalCount, page, pageSize), nil
}

func (uc *ListTenantsUseCase) GetByType(ctx context.Context, tenantType value_object.TenantType, page, pageSize int) (*response.TenantListResponse, error) {
	// Calcular offset
	offset := (page - 1) * pageSize

	// Obtener tenants por tipo
	tenants, err := uc.tenantRepo.GetByType(ctx, tenantType, pageSize, offset)
	if err != nil {
		return nil, err
	}

	// Para obtener el total, usamos Count general ya que no tenemos CountByType en la interfaz
	// Esto es una simplificación, en producción podrías agregarlo
	allTenants, err := uc.tenantRepo.GetByType(ctx, tenantType, -1, 0)
	if err != nil {
		return nil, err
	}

	return response.NewTenantListResponse(tenants, len(allTenants), page, pageSize), nil
}

func (uc *ListTenantsUseCase) GetActive(ctx context.Context, page, pageSize int) (*response.TenantListResponse, error) {
	// Calcular offset
	offset := (page - 1) * pageSize

	// Obtener tenants activos
	tenants, err := uc.tenantRepo.GetActive(ctx, pageSize, offset)
	if err != nil {
		return nil, err
	}

	// Obtener total activos
	totalCount, err := uc.tenantRepo.CountByStatus(ctx, value_object.TenantStatusActive)
	if err != nil {
		return nil, err
	}

	return response.NewTenantListResponse(tenants, totalCount, page, pageSize), nil
}

func (uc *ListTenantsUseCase) GetExpiring(ctx context.Context, days int) (*response.TenantListResponse, error) {
	tenants, err := uc.tenantRepo.GetExpiring(ctx, days)
	if err != nil {
		return nil, err
	}

	return response.NewTenantListResponse(tenants, len(tenants), 1, len(tenants)), nil
}
