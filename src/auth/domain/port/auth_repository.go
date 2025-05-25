package port

import (
	"context"
	"iam/src/auth/domain/entity"
	"iam/src/auth/domain/value_object"

	"github.com/google/uuid"
)

type AuthRepository interface {
	// Refresh Tokens
	CreateRefreshToken(ctx context.Context, token *entity.RefreshToken) error
	GetRefreshToken(ctx context.Context, token string) (*entity.RefreshToken, error)
	DeleteRefreshToken(ctx context.Context, token string) error
	DeleteAllUserRefreshTokens(ctx context.Context, userID uuid.UUID) error

	// Federated Auth - estos necesitan acceso a User, así que podríamos usar shared o interface
	GetUserByFederatedID(ctx context.Context, provider value_object.AuthProvider, federatedID string, tenantID *uuid.UUID) (UserData, error)
	LinkFederatedID(ctx context.Context, userID uuid.UUID, provider value_object.AuthProvider, federatedID string) error
}
