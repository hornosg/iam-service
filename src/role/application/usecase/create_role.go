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

	// Obtener tenant ID si se proporciona
	tenantID, err := req.GetTenantID()
	if err != nil {
		return nil, exception.ErrInvalidTenant
	}

	// Verificar que no existe un rol con el mismo nombre en el mismo tenant
	exists, err := uc.roleRepo.ExistsByName(ctx, req.Name, tenantID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, exception.ErrRoleAlreadyExists
	}

	// Crear la entidad
	role := entity.NewRole(req.Name, req.Description, roleType, tenantID)

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
