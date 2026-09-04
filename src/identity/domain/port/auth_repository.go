package port

import (
	"context"
	"errors"
	"iam/src/identity/domain/entity"
	"iam/src/identity/domain/value_object"
	"time"

	"github.com/google/uuid"
)

// ErrRefreshTokenAlreadyConsumed: la fila del refresh token ya no existe al
// momento de eliminarla — la rotación single-use de otro request la consumió
// primero (ACC-E02 T8f). Es un hecho del dominio, no de infraestructura: el
// caso de uso lo traduce a credencial inválida (401), nunca a 500. El repo lo
// devuelve envuelto (`fmt.Errorf("%w: ...", ErrRefreshTokenAlreadyConsumed)`)
// para que el caller distinga con errors.Is.
var ErrRefreshTokenAlreadyConsumed = errors.New("refresh token ya consumido")

type AuthRepository interface {
	// Refresh Tokens
	CreateRefreshToken(ctx context.Context, token *entity.RefreshToken) error
	GetRefreshToken(ctx context.Context, token string) (*entity.RefreshToken, error)
	DeleteRefreshToken(ctx context.Context, token string) error
	DeleteAllUserRefreshTokens(ctx context.Context, userID uuid.UUID) error

	// Token Revocation
	RevokeToken(ctx context.Context, jti uuid.UUID, userID uuid.UUID, expiresAt time.Time) error
	// IsTokenRevoked verifica si el token presentado quedó revocado, por JTI
	// (logout de una sesión) o por marca de alcance user de revoke-all:
	// revocado si existe una marca scope='user' de su usuario con
	// revoked_at > issuedAt — todo token emitido antes del corte (ACC-E02
	// T8i). Un token legacy sin iat (issuedAt=0) queda revocado por cualquier
	// marca viva: fail-closed, no se puede saber cuándo fue emitido.
	IsTokenRevoked(ctx context.Context, jti uuid.UUID, userID uuid.UUID, issuedAt int64) (bool, error)
	RevokeAllUserTokens(ctx context.Context, userID uuid.UUID, expiresAt time.Time) error
	CleanupExpiredRevocations(ctx context.Context) (int64, error)

	// Federated Auth
	GetUserByFederatedID(ctx context.Context, provider value_object.AuthProvider, federatedID string, tenantID *uuid.UUID) (UserData, error)
	LinkFederatedID(ctx context.Context, userID uuid.UUID, provider value_object.AuthProvider, federatedID string) error
}
