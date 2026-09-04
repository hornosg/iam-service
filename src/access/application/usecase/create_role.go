package usecase

import (
	"context"

	"iam/src/access/application/request"
	"iam/src/access/application/response"
	"iam/src/access/domain/entity"
	"iam/src/access/domain/exception"
	"iam/src/access/domain/port"
)

type CreateRoleUseCase struct {
	roleRepo port.RoleRepository
}

func NewCreateRoleUseCase(roleRepo port.RoleRepository) *CreateRoleUseCase {
	return &CreateRoleUseCase{
		roleRepo: roleRepo,
	}
}

func (uc *CreateRoleUseCase) Execute(ctx context.Context, req *request.CreateRoleRequest) (*response.RoleResponse, error) {
	// Obtener tipo de rol
	roleType, err := req.GetRoleType()
	if err != nil {
		return nil, exception.ErrInvalidRoleType
	}

	// ACC-E02 T10: `roles` es un catálogo global sin tenant_id. La unicidad de
	// nombre es global (constraint roles_name_unique).
	exists, err := uc.roleRepo.ExistsByName(ctx, req.Name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, exception.ErrRoleAlreadyExists
	}

	// Crear la entidad (sin tenant: roles globales)
	role := entity.NewRole(req.Name, req.Description, roleType)

	// Agregar permisos si se proporcionaron
	for _, permission := range req.Permissions {
		role.AddPermission(permission)
	}

	// Guardar en repositorio
	if err := uc.roleRepo.Create(ctx, role); err != nil {
		return nil, err
	}

	return response.NewRoleResponse(role), nil
}
