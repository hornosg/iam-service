package usecase

import (
	"context"

	"iam/src/role/application/request"
	"iam/src/role/application/response"
	"iam/src/role/domain/entity"
	"iam/src/role/domain/exception"
	"iam/src/role/domain/port"
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
	// nombre es global (constraint roles_name_unique), por lo que ExistsByName
	// no recibe tenant.
	exists, err := uc.roleRepo.ExistsByName(ctx, req.Name, nil)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, exception.ErrRoleAlreadyExists
	}

	// Crear la entidad (sin tenant: roles globales)
	role := entity.NewRole(req.Name, req.Description, roleType, nil)

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
