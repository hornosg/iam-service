package usecase

import (
	"context"
	"github.com/hornosg/go-shared/criteria"
	"iam/src/role/domain/entity"
	"iam/src/role/domain/port"
)

// ListRolesByCriteriaUseCase lista roles usando criterios
type ListRolesByCriteriaUseCase struct {
	roleRepo port.RoleCriteriaRepository
}

// NewListRolesByCriteriaUseCase crea una nueva instancia del caso de uso
func NewListRolesByCriteriaUseCase(roleRepo port.RoleCriteriaRepository) *ListRolesByCriteriaUseCase {
	return &ListRolesByCriteriaUseCase{
		roleRepo: roleRepo,
	}
}

// Execute ejecuta el caso de uso
func (uc *ListRolesByCriteriaUseCase) Execute(ctx context.Context, searchCriteria criteria.Criteria) (*criteria.ListResponse[entity.Role], error) {
	// Buscar roles usando criterios
	roles, err := uc.roleRepo.SearchByCriteria(ctx, searchCriteria)
	if err != nil {
		return nil, err
	}

	// Contar total de roles con los mismos filtros (sin paginación)
	countCriteria := criteria.Criteria{
		Filters: searchCriteria.Filters,
		// No incluir Order ni Pagination para el conteo
	}
	total, err := uc.roleRepo.CountByCriteria(ctx, countCriteria)
	if err != nil {
		return nil, err
	}

	// Crear respuesta con información de paginación
	return criteria.NewListResponseFromCriteria(roles, total, searchCriteria), nil
}
