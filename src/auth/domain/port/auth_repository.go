package port

import (
	"context"
	"iam/src/auth/domain/entity"
	"iam/src/auth/domain/value_object"
	"time"

	"github.com/google/uuid"
)

type AuthRepository interface {
	// Refresh Tokens
	CreateRefreshToken(ctx context.Context, token *entity.RefreshToken) error
	GetRefreshToken(ctx context.Context, token string) (*entity.RefreshToken, error)
	DeleteRefreshToken(ctx context.Context, token string) error
	DeleteAllUserRefreshTokens(ctx context.Context, userID uuid.UUID) error

	// Token Revocation
	RevokeToken(ctx context.Context, jti uuid.UUID, userID uuid.UUID, expiresAt time.Time) error
	IsTokenRevoked(ctx context.Context, jti uuid.UUID) (bool, error)
	RevokeAllUserTokens(ctx context.Context, userID uuid.UUID, expiresAt time.Time) error
	CleanupExpiredRevocations(ctx context.Context) (int64, error)

	// Federated Auth
	GetUserByFederatedID(ctx context.Context, provider value_object.AuthProvider, federatedID string, tenantID *uuid.UUID) (UserData, error)
	LinkFederatedID(ctx context.Context, userID uuid.UUID, provider value_object.AuthProvider, federatedID string) error
}
