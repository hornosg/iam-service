package usecase

import (
	"context"

	"iam/src/role/application/response"
	"iam/src/role/domain/port"

	"github.com/google/uuid"
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

func (uc *ListRolesUseCase) GetByTenant(ctx context.Context, tenantID uuid.UUID, page, pageSize int) (*response.RoleListResponse, error) {
	// Calcular offset
	offset := (page - 1) * pageSize

	// Obtener roles por tenant
	roles, err := uc.roleRepo.GetByTenant(ctx, tenantID, pageSize, offset)
	if err != nil {
		return nil, err
	}

	// Obtener total por tenant
	totalCount, err := uc.roleRepo.CountByTenant(ctx, tenantID)
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

func (uc *ListRolesUseCase) GetActiveRoles(ctx context.Context, tenantID *uuid.UUID, page, pageSize int) (*response.RoleListResponse, error) {
	// Calcular offset
	offset := (page - 1) * pageSize

	// Obtener roles activos
	roles, err := uc.roleRepo.GetActiveRoles(ctx, tenantID, pageSize, offset)
	if err != nil {
		return nil, err
	}

	// Para el total, usamos la misma consulta pero sin límite
	allActiveRoles, err := uc.roleRepo.GetActiveRoles(ctx, tenantID, -1, 0)
	if err != nil {
		return nil, err
	}

	return response.NewRoleListResponse(roles, len(allActiveRoles), page, pageSize), nil
}
