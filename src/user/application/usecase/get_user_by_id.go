package usecase

import (
	"context"
	"iam/src/user/application/response"
	"iam/src/user/domain/exception"
	"iam/src/user/domain/port"

	"github.com/google/uuid"
)

type GetUserByIDUseCase struct {
	userRepo port.UserRepository
}

func NewGetUserByIDUseCase(userRepo port.UserRepository) *GetUserByIDUseCase {
	return &GetUserByIDUseCase{
		userRepo: userRepo,
	}
}

func (uc *GetUserByIDUseCase) Execute(ctx context.Context, id uuid.UUID) (*response.UserResponse, error) {
	user, err := uc.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, exception.ErrUserNotFound
	}

	return response.NewUserResponse(user), nil
}
