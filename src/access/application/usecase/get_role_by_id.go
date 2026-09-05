package usecase

import (
	"context"

	"github.com/hornosg/iam-service/src/access/application/response"
	"github.com/hornosg/iam-service/src/access/domain/port"

	"github.com/google/uuid"
)

type GetRoleByIDUseCase struct {
	roleRepo port.RoleRepository
}

func NewGetRoleByIDUseCase(roleRepo port.RoleRepository) *GetRoleByIDUseCase {
	return &GetRoleByIDUseCase{
		roleRepo: roleRepo,
	}
}

func (uc *GetRoleByIDUseCase) Execute(ctx context.Context, id uuid.UUID) (*response.RoleResponse, error) {
	role, err := uc.roleRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return response.NewRoleResponse(role), nil
}
