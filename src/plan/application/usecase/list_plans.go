package usecase

import (
	"context"

	"iam/src/plan/application/response"
	"iam/src/plan/domain/port"
)

type ListPlansUseCase struct {
	planRepo port.PlanRepository
}

func NewListPlansUseCase(planRepo port.PlanRepository) *ListPlansUseCase {
	return &ListPlansUseCase{
		planRepo: planRepo,
	}
}

func (uc *ListPlansUseCase) Execute(ctx context.Context, page, pageSize int) (*response.PlanListResponse, error) {
	// Calcular offset
	offset := (page - 1) * pageSize

	// Obtener planes
	plans, err := uc.planRepo.List(ctx, pageSize, offset)
	if err != nil {
		return nil, err
	}

	// Obtener total
	totalCount, err := uc.planRepo.Count(ctx)
	if err != nil {
		return nil, err
	}

	return response.NewPlanListResponse(plans, totalCount, page, pageSize), nil
}

func (uc *ListPlansUseCase) GetActive(ctx context.Context) (*response.PlanListResponse, error) {
	plans, err := uc.planRepo.GetActive(ctx)
	if err != nil {
		return nil, err
	}

	return response.NewPlanListResponse(plans, len(plans), 1, len(plans)), nil
}
