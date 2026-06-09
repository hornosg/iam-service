package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"

	"iam/src/auth/domain/port"
	"iam/src/auth/domain/value_object"
	sharedport "github.com/mercadocercano/go-shared/domain/port"
)

type LogoutUseCase struct {
	authRepo       port.AuthRepository
	securityLogger sharedport.SecurityEventLogger
}

func NewLogoutUseCase(authRepo port.AuthRepository, securityLogger sharedport.SecurityEventLogger) *LogoutUseCase {
	return &LogoutUseCase{
		authRepo:       authRepo,
		securityLogger: securityLogger,
	}
}

func (uc *LogoutUseCase) Execute(ctx context.Context, userID uuid.UUID, claims *value_object.TokenClaims) error {
	if claims != nil && claims.JTI != uuid.Nil {
		expiresAt := time.Unix(claims.ExpiresAt, 0)
		_ = uc.authRepo.RevokeToken(ctx, claims.JTI, userID, expiresAt)
	}

	tenantID := ""
	if claims != nil {
		tenantID = claims.TenantID.String()
	}
	uc.securityLogger.Log(sharedport.SecurityEvent{
		Event:    sharedport.EventLogout,
		UserID:   userID.String(),
		TenantID: tenantID,
	})

	return uc.authRepo.DeleteAllUserRefreshTokens(ctx, userID)
}
