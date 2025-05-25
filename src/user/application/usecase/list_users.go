package usecase

import (
	"context"
	"errors"
	"iam/src/user/application/response"
	"iam/src/user/domain/entity"
	"iam/src/user/domain/port"
	"iam/src/user/domain/value_object"

	"github.com/google/uuid"
)

type ListUsersUseCase struct {
	userRepo port.UserRepository
}

func NewListUsersUseCase(userRepo port.UserRepository) *ListUsersUseCase {
	return &ListUsersUseCase{
		userRepo: userRepo,
	}
}

type ListUsersParams struct {
	TenantID *uuid.UUID               `json:"tenant_id,omitempty"`
	Status   *value_object.UserStatus `json:"status,omitempty"`
	RoleID   *uuid.UUID               `json:"role_id,omitempty"`
	Page     int                      `json:"page"`
	PageSize int                      `json:"page_size"`
}

func (p *ListUsersParams) GetOffset() int {
	if p.Page <= 1 {
		return 0
	}
	return (p.Page - 1) * p.PageSize
}

func (p *ListUsersParams) GetLimit() int {
	if p.PageSize <= 0 {
		return 10 // Default page size
	}
	if p.PageSize > 100 {
		return 100 // Max page size
	}
	return p.PageSize
}

func (uc *ListUsersUseCase) Execute(ctx context.Context, params *ListUsersParams) (*response.UserListResponse, error) {
	limit := params.GetLimit()
	offset := params.GetOffset()

	var users []*entity.User
	var total int
	var err error

	// Buscar según los filtros proporcionados
	if params.TenantID != nil {
		users, err = uc.userRepo.GetByTenant(ctx, *params.TenantID, limit, offset)
		if err != nil {
			return nil, err
		}
		total, err = uc.userRepo.CountByTenant(ctx, *params.TenantID)
	} else if params.Status != nil {
		users, err = uc.userRepo.GetByStatus(ctx, *params.Status, limit, offset)
		if err != nil {
			return nil, err
		}
		total, err = uc.userRepo.CountByStatus(ctx, *params.Status)
	} else if params.RoleID != nil {
		users, err = uc.userRepo.GetByRole(ctx, *params.RoleID, limit, offset)
		if err != nil {
			return nil, err
		}
		// Para role no tenemos count específico, usamos un estimate
		total = len(users) // Este sería un aproximado, idealmente agregaríamos CountByRole
	} else {
		return nil, errors.New("al menos un filtro es requerido (tenant_id, status, o role_id)")
	}

	if err != nil {
		return nil, err
	}

	return response.NewUserListResponse(users, total, params.Page, limit), nil
}
