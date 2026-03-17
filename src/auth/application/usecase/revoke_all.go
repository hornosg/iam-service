package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"

	"iam/src/auth/domain/port"
	"iam/src/auth/infrastructure/logging"
)

type RevokeAllUseCase struct {
	authRepo          port.AuthRepository
	accessTokenExpiry time.Duration
	securityLogger    *logging.SecurityLogger
}

func NewRevokeAllUseCase(authRepo port.AuthRepository, accessTokenExpiry time.Duration) *RevokeAllUseCase {
	return &RevokeAllUseCase{
		authRepo:          authRepo,
		accessTokenExpiry: accessTokenExpiry,
		securityLogger:    logging.NewSecurityLogger(),
	}
}

func (uc *RevokeAllUseCase) Execute(ctx context.Context, userID uuid.UUID) error {
	expiresAt := time.Now().Add(uc.accessTokenExpiry)

	if err := uc.authRepo.RevokeAllUserTokens(ctx, userID, expiresAt); err != nil {
		return err
	}

	uc.securityLogger.LogTokenRevoked(userID.String(), "", "", "all")

	return uc.authRepo.DeleteAllUserRefreshTokens(ctx, userID)
}
