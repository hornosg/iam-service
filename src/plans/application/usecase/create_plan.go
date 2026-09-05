package usecase

import (
	"context"

	"github.com/hornosg/iam-service/src/plans/application/request"
	"github.com/hornosg/iam-service/src/plans/application/response"
	"github.com/hornosg/iam-service/src/plans/domain/entity"
	"github.com/hornosg/iam-service/src/plans/domain/exception"
	"github.com/hornosg/iam-service/src/plans/domain/port"
)

type CreatePlanUseCase struct {
	planRepo port.PlanRepository
}

func NewCreatePlanUseCase(planRepo port.PlanRepository) *CreatePlanUseCase {
	return &CreatePlanUseCase{
		planRepo: planRepo,
	}
}

func (uc *CreatePlanUseCase) Execute(ctx context.Context, req *request.CreatePlanRequest) (*response.PlanResponse, error) {
	// Verificar que no existe un plan con el mismo nombre
	exists, err := uc.planRepo.ExistsByName(ctx, req.Name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, exception.ErrPlanAlreadyExists
	}

	// Obtener tipo de plan
	planType, err := req.GetPlanType()
	if err != nil {
		return nil, exception.ErrInvalidPlanType
	}

	// Crear la entidad
	plan := entity.NewPlan(req.Name, req.Description, planType, req.PriceMonth, req.PriceYear)

	// Agregar features si se proporcionaron
	for _, feature := range req.Features {
		plan.AddFeature(feature)
	}

	// Guardar en repositorio
	if err := uc.planRepo.Create(ctx, plan); err != nil {
		return nil, err
	}

	return response.NewPlanResponse(plan), nil
}
