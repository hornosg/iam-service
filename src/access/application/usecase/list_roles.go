package usecase

import (
	"context"

	"github.com/hornosg/iam-service/src/access/application/response"
	"github.com/hornosg/iam-service/src/access/domain/port"
)

type ListRolesUseCase struct {
	roleRepo port.RoleRepository
}

func NewListRolesUseCase(roleRepo port.RoleRepository) *ListRolesUseCase {
	return &ListRolesUseCase{
		roleRepo: roleRepo,
	}
}

func (uc *ListRolesUseCase) Execute(ctx context.Context, page, pageSize int) (*response.RoleListResponse, error) {
	// Calcular offset
	offset := (page - 1) * pageSize

	// Obtener roles
	roles, err := uc.roleRepo.List(ctx, pageSize, offset)
	if err != nil {
		return nil, err
	}

	// Obtener total
	totalCount, err := uc.roleRepo.Count(ctx)
	if err != nil {
		return nil, err
	}

	return response.NewRoleListResponse(roles, totalCount, page, pageSize), nil
}

func (uc *ListRolesUseCase) GetSystemRoles(ctx context.Context) (*response.RoleListResponse, error) {
	roles, err := uc.roleRepo.GetSystemRoles(ctx)
	if err != nil {
		return nil, err
	}

	return response.NewRoleListResponse(roles, len(roles), 1, len(roles)), nil
}

func (uc *ListRolesUseCase) GetActiveRoles(ctx context.Context, page, pageSize int) (*response.RoleListResponse, error) {
	// Calcular offset
	offset := (page - 1) * pageSize

	// Obtener roles activos
	roles, err := uc.roleRepo.GetActiveRoles(ctx, pageSize, offset)
	if err != nil {
		return nil, err
	}

	// Para el total, usamos la misma consulta pero sin límite
	allActiveRoles, err := uc.roleRepo.GetActiveRoles(ctx, -1, 0)
	if err != nil {
		return nil, err
	}

	return response.NewRoleListResponse(roles, len(allActiveRoles), page, pageSize), nil
}
