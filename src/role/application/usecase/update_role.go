package usecase

import (
	"context"

	"iam/src/role/application/request"
	"iam/src/role/application/response"
	"iam/src/role/domain/exception"
	"iam/src/role/domain/port"
	"iam/src/role/domain/value_object"

	"github.com/google/uuid"
)

type UpdateRoleUseCase struct {
	roleRepo port.RoleRepository
}

func NewUpdateRoleUseCase(roleRepo port.RoleRepository) *UpdateRoleUseCase {
	return &UpdateRoleUseCase{
		roleRepo: roleRepo,
	}
}

func (uc *UpdateRoleUseCase) Execute(ctx context.Context, id uuid.UUID, req *request.UpdateRoleRequest) (*response.RoleResponse, error) {
	// Obtener rol existente
	role, err := uc.roleRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Verificar si es un rol del sistema (no debería modificarse)
	if role.Type == value_object.RoleTypeSystemAdmin {
		return nil, exception.ErrSystemRoleModification
	}

	// Actualizar campos si se proporcionan
	if req.Name != nil && req.Description != nil {
		role.UpdateDetails(*req.Name, *req.Description)
	}

	// Actualizar estado de activación
	if req.IsActive != nil {
		if *req.IsActive {
			role.Activate()
		} else {
			role.Deactivate()
		}
	}

	// Actualizar permisos si se proporcionan
	if req.Permissions != nil {
		// Limpiar permisos actuales y agregar los nuevos
		role.Permissions = []string{}
		for _, permission := range *req.Permissions {
			role.AddPermission(permission)
		}
	}

	// Guardar cambios
	if err := uc.roleRepo.Update(ctx, role); err != nil {
		return nil, err
	}

	return response.NewRoleResponse(role), nil
}
