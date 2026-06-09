package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"

	"iam/src/auth/domain/port"
	sharedport "github.com/mercadocercano/go-shared/domain/port"
)

type RevokeAllUseCase struct {
	authRepo          port.AuthRepository
	accessTokenExpiry time.Duration
	securityLogger    sharedport.SecurityEventLogger
}

func NewRevokeAllUseCase(authRepo port.AuthRepository, accessTokenExpiry time.Duration, securityLogger sharedport.SecurityEventLogger) *RevokeAllUseCase {
	return &RevokeAllUseCase{
		authRepo:          authRepo,
		accessTokenExpiry: accessTokenExpiry,
		securityLogger:    securityLogger,
	}
}

func (uc *RevokeAllUseCase) Execute(ctx context.Context, userID uuid.UUID) error {
	expiresAt := time.Now().Add(uc.accessTokenExpiry)

	if err := uc.authRepo.RevokeAllUserTokens(ctx, userID, expiresAt); err != nil {
		return err
	}

	uc.securityLogger.Log(sharedport.SecurityEvent{
		Event:  sharedport.EventTokenRevoked,
		UserID: userID.String(),
		Scope:  "all",
	})

	return uc.authRepo.DeleteAllUserRefreshTokens(ctx, userID)
}
