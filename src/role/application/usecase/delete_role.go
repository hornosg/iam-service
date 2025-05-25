package usecase

import (
	"context"

	"iam/src/role/domain/exception"
	"iam/src/role/domain/port"
	"iam/src/role/domain/value_object"

	"github.com/google/uuid"
)

type DeleteRoleUseCase struct {
	roleRepo port.RoleRepository
}

func NewDeleteRoleUseCase(roleRepo port.RoleRepository) *DeleteRoleUseCase {
	return &DeleteRoleUseCase{
		roleRepo: roleRepo,
	}
}

func (uc *DeleteRoleUseCase) Execute(ctx context.Context, id uuid.UUID) error {
	// Obtener rol existente
	role, err := uc.roleRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Verificar si es un rol del sistema (no debería eliminarse)
	if role.Type == value_object.RoleTypeSystemAdmin ||
		role.Type == value_object.RoleTypeTenantAdmin ||
		role.Type == value_object.RoleTypeUser ||
		role.Type == value_object.RoleTypeReadOnly {
		return exception.ErrCannotDeleteRole
	}

	// Eliminar rol (soft delete desactivando)
	role.Deactivate()
	if err := uc.roleRepo.Update(ctx, role); err != nil {
		return err
	}

	return nil
}
