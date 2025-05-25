package usecase

import (
	"context"

	"iam/src/tenant/domain/exception"
	"iam/src/tenant/domain/port"

	"github.com/google/uuid"
)

type DeleteTenantUseCase struct {
	tenantRepo port.TenantRepository
}

func NewDeleteTenantUseCase(tenantRepo port.TenantRepository) *DeleteTenantUseCase {
	return &DeleteTenantUseCase{
		tenantRepo: tenantRepo,
	}
}

func (uc *DeleteTenantUseCase) Execute(ctx context.Context, id uuid.UUID) error {
	// Obtener tenant existente
	tenant, err := uc.tenantRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Verificar que el tenant puede ser eliminado
	if !tenant.CanBeModified() {
		return exception.ErrCannotDeleteTenant
	}

	// Verificar si tiene usuarios activos (opcional: puedes agregar esta validación)
	if tenant.UserCount > 0 {
		return exception.ErrCannotDeleteTenant
	}

	// Realizar soft delete
	tenant.Delete()

	// Guardar cambios
	if err := uc.tenantRepo.Update(ctx, tenant); err != nil {
		return err
	}

	return nil
}
