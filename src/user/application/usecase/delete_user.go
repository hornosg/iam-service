package usecase

import (
	"context"
	"iam/src/user/domain/exception"
	"iam/src/user/domain/port"

	"github.com/google/uuid"
)

type DeleteUserUseCase struct {
	userRepo port.UserRepository
}

func NewDeleteUserUseCase(userRepo port.UserRepository) *DeleteUserUseCase {
	return &DeleteUserUseCase{
		userRepo: userRepo,
	}
}

func (uc *DeleteUserUseCase) Execute(ctx context.Context, id uuid.UUID) error {
	// Verificar que el usuario existe
	_, err := uc.userRepo.GetByID(ctx, id)
	if err != nil {
		return exception.ErrUserNotFound
	}

	// Eliminar usuario
	return uc.userRepo.Delete(ctx, id)
}
