package usecase

import (
	"context"

	"iam/src/tenant/application/request"
	"iam/src/tenant/application/response"
	"iam/src/tenant/domain/exception"
	"iam/src/tenant/domain/port"

	"github.com/google/uuid"
)

type SetPlanUseCase struct {
	tenantRepo port.TenantRepository
}

func NewSetPlanUseCase(tenantRepo port.TenantRepository) *SetPlanUseCase {
	return &SetPlanUseCase{
		tenantRepo: tenantRepo,
	}
}

func (uc *SetPlanUseCase) Execute(ctx context.Context, tenantID uuid.UUID, req *request.SetPlanRequest) (*response.TenantResponse, error) {
	// Obtener tenant existente
	tenant, err := uc.tenantRepo.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// Verificar que el tenant puede ser modificado
	if !tenant.CanBeModified() {
		return nil, exception.ErrTenantDeleted
	}

	// Verificar que el tenant está activo
	if !tenant.IsActive() {
		return nil, exception.ErrTenantNotActive
	}

	// Obtener plan ID
	planID, err := req.GetPlanID()
	if err != nil {
		return nil, exception.ErrPlanNotFound
	}

	// TODO: Aquí podrías verificar que el plan existe en el repositorio de planes
	// planExists, err := uc.planRepo.ExistsByID(ctx, planID)
	// if err != nil {
	//     return nil, err
	// }
	// if !planExists {
	//     return nil, exception.ErrPlanNotFound
	// }

	// Asignar plan al tenant
	tenant.SetPlan(planID)

	// Guardar cambios
	if err := uc.tenantRepo.Update(ctx, tenant); err != nil {
		return nil, err
	}

	return response.NewTenantResponse(tenant), nil
}

func (uc *SetPlanUseCase) RemovePlan(ctx context.Context, tenantID uuid.UUID) (*response.TenantResponse, error) {
	// Obtener tenant existente
	tenant, err := uc.tenantRepo.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// Verificar que el tenant puede ser modificado
	if !tenant.CanBeModified() {
		return nil, exception.ErrTenantDeleted
	}

	// Remover plan del tenant
	tenant.RemovePlan()

	// Guardar cambios
	if err := uc.tenantRepo.Update(ctx, tenant); err != nil {
		return nil, err
	}

	return response.NewTenantResponse(tenant), nil
}
