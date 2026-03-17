package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"

	"iam/src/auth/domain/port"
	"iam/src/auth/domain/value_object"
	"iam/src/auth/infrastructure/logging"
)

type LogoutUseCase struct {
	authRepo       port.AuthRepository
	securityLogger *logging.SecurityLogger
}

func NewLogoutUseCase(authRepo port.AuthRepository) *LogoutUseCase {
	return &LogoutUseCase{
		authRepo:       authRepo,
		securityLogger: logging.NewSecurityLogger(),
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
	uc.securityLogger.LogLogout(userID.String(), tenantID, "")

	return uc.authRepo.DeleteAllUserRefreshTokens(ctx, userID)
}
