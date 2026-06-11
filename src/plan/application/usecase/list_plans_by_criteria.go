package usecase

import (
	"context"
	"github.com/hornosg/go-shared/criteria"
	"iam/src/plan/domain/entity"
	"iam/src/plan/domain/port"
)

// ListPlansByCriteriaUseCase lista planes usando criterios
type ListPlansByCriteriaUseCase struct {
	planRepo port.PlanCriteriaRepository
}

// NewListPlansByCriteriaUseCase crea una nueva instancia del caso de uso
func NewListPlansByCriteriaUseCase(planRepo port.PlanCriteriaRepository) *ListPlansByCriteriaUseCase {
	return &ListPlansByCriteriaUseCase{
		planRepo: planRepo,
	}
}

// Execute ejecuta el caso de uso
func (uc *ListPlansByCriteriaUseCase) Execute(ctx context.Context, searchCriteria criteria.Criteria) (*criteria.ListResponse[entity.Plan], error) {
	// Buscar planes usando criterios
	plans, err := uc.planRepo.SearchByCriteria(ctx, searchCriteria)
	if err != nil {
		return nil, err
	}

	// Contar total de planes con los mismos filtros (sin paginación)
	countCriteria := criteria.Criteria{
		Filters: searchCriteria.Filters,
		// No incluir Order ni Pagination para el conteo
	}
	total, err := uc.planRepo.CountByCriteria(ctx, countCriteria)
	if err != nil {
		return nil, err
	}

	// Crear respuesta con información de paginación
	return criteria.NewListResponseFromCriteria(plans, total, searchCriteria), nil
}
