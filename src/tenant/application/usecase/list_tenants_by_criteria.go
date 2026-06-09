package usecase

import (
	"context"
	"github.com/mercadocercano/criteria"
	"iam/src/tenant/domain/entity"
	"iam/src/tenant/domain/port"
)

// ListTenantsByCriteriaUseCase lista tenants usando criterios
type ListTenantsByCriteriaUseCase struct {
	tenantRepo port.TenantCriteriaRepository
}

// NewListTenantsByCriteriaUseCase crea una nueva instancia del caso de uso
func NewListTenantsByCriteriaUseCase(tenantRepo port.TenantCriteriaRepository) *ListTenantsByCriteriaUseCase {
	return &ListTenantsByCriteriaUseCase{
		tenantRepo: tenantRepo,
	}
}

// Execute ejecuta el caso de uso
func (uc *ListTenantsByCriteriaUseCase) Execute(ctx context.Context, searchCriteria criteria.Criteria) (*criteria.ListResponse[entity.Tenant], error) {
	// Buscar tenants usando criterios
	tenants, err := uc.tenantRepo.SearchByCriteria(ctx, searchCriteria)
	if err != nil {
		return nil, err
	}

	// Contar total de tenants con los mismos filtros (sin paginación)
	countCriteria := criteria.Criteria{
		Filters: searchCriteria.Filters,
		// No incluir Order ni Pagination para el conteo
	}
	total, err := uc.tenantRepo.CountByCriteria(ctx, countCriteria)
	if err != nil {
		return nil, err
	}

	// Crear respuesta con información de paginación
	return criteria.NewListResponseFromCriteria(tenants, total, searchCriteria), nil
}
