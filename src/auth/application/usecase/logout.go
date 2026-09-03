package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"

	"iam/src/auth/domain/port"
	"iam/src/auth/domain/value_object"
	sharedctx "iam/src/shared/context"
	sharedport "github.com/hornosg/go-shared/domain/port"
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
	// ACC-E02 T8e: las escrituras de revocación corren bajo account_app con RLS;
	// el tenant de los claims (ya verificados por el gate de revocación) es el
	// que las policies de revoked_tokens/refresh_tokens exigen. Sin él, el repo
	// rechaza la operación (fail-closed explícito).
	if claims != nil && claims.TenantID != uuid.Nil {
		ctx = sharedctx.WithTenantID(ctx, claims.TenantID)
	}

	if claims != nil && claims.JTI != uuid.Nil {
		expiresAt := time.Unix(claims.ExpiresAt, 0)
		// T8e: ya no se ignora el error. Antes el INSERT violaba el WITH CHECK
		// de la policy (sin app.tenant_id) y el error se descartaba acá: el
		// logout respondía 204 sin haber revocado nada.
		if err := uc.authRepo.RevokeToken(ctx, claims.JTI, userID, expiresAt); err != nil {
			return err
		}
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
