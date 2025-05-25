package usecase

import (
	"context"

	"iam/src/plan/application/response"
	"iam/src/plan/domain/port"

	"github.com/google/uuid"
)

type GetPlanByIDUseCase struct {
	planRepo port.PlanRepository
}

func NewGetPlanByIDUseCase(planRepo port.PlanRepository) *GetPlanByIDUseCase {
	return &GetPlanByIDUseCase{
		planRepo: planRepo,
	}
}

func (uc *GetPlanByIDUseCase) Execute(ctx context.Context, id uuid.UUID) (*response.PlanResponse, error) {
	plan, err := uc.planRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return response.NewPlanResponse(plan), nil
}
