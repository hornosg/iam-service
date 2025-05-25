package usecase

import (
	"context"

	"github.com/google/uuid"

	"iam/src/auth/domain/port"
)

type LogoutUseCase struct {
	authRepo port.AuthRepository
}

func NewLogoutUseCase(authRepo port.AuthRepository) *LogoutUseCase {
	return &LogoutUseCase{
		authRepo: authRepo,
	}
}

func (uc *LogoutUseCase) Execute(ctx context.Context, userID uuid.UUID) error {
	// Eliminar todos los refresh tokens del usuario
	return uc.authRepo.DeleteAllUserRefreshTokens(ctx, userID)
}
