package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"iam/src/auth/domain/entity"
	"iam/src/auth/domain/port"
	"iam/src/auth/domain/value_object"
	sharedctx "iam/src/shared/context"
	sharedpostgres "iam/src/shared/postgres"
)

type PostgresAuthRepository struct {
	db *sql.DB
}

func NewPostgresAuthRepository(db *sql.DB) port.AuthRepository {
	return &PostgresAuthRepository{
		db: db,
	}
}

type execer interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

// CreateRefreshToken almacena un nuevo refresh token. Si el contexto porta un
// tenant_id, la inserción corre bajo account_app con SET LOCAL app.tenant_id
// para que la policy RLS de refresh_tokens permita la fila (ACC-E02 T4/T5).
func (r *PostgresAuthRepository) CreateRefreshToken(ctx context.Context, token *entity.RefreshToken) error {
	if tenantID, ok := sharedctx.TenantIDFromContext(ctx); ok {
		return sharedpostgres.WithRLSInTransaction(ctx, r.db, tenantID, func(ctx context.Context, tx *sql.Tx) error {
			return r.createRefreshToken(ctx, tx, token)
		})
	}
	return r.createRefreshToken(ctx, r.db, token)
}

func (r *PostgresAuthRepository) createRefreshToken(ctx context.Context, db execer, token *entity.RefreshToken) error {
	query := `
		INSERT INTO refresh_tokens (id, user_id, token, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5)`

	_, err := db.ExecContext(ctx, query,
		token.ID,
		token.UserID,
		token.Token,
		token.ExpiresAt,
		token.CreatedAt,
	)

	if err != nil {
		return fmt.Errorf("error creando refresh token: %w", err)
	}

	return nil
}

// GetRefreshToken obtiene un refresh token por su valor
func (r *PostgresAuthRepository) GetRefreshToken(ctx context.Context, token string) (*entity.RefreshToken, error) {
	query := `
		SELECT id, user_id, token, expires_at, created_at 
		FROM refresh_tokens 
		WHERE token = $1`

	var refreshToken entity.RefreshToken
	err := r.db.QueryRowContext(ctx, query, token).Scan(
		&refreshToken.ID,
		&refreshToken.UserID,
		&refreshToken.Token,
		&refreshToken.ExpiresAt,
		&refreshToken.CreatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("refresh token no encontrado")
		}
		return nil, fmt.Errorf("error obteniendo refresh token: %w", err)
	}

	return &refreshToken, nil
}

// DeleteRefreshToken elimina un refresh token específico
func (r *PostgresAuthRepository) DeleteRefreshToken(ctx context.Context, token string) error {
	query := `DELETE FROM refresh_tokens WHERE token = $1`

	_, err := r.db.ExecContext(ctx, query, token)
	if err != nil {
		return fmt.Errorf("error eliminando refresh token: %w", err)
	}

	return nil
}

// DeleteAllUserRefreshTokens elimina todos los refresh tokens de un usuario
func (r *PostgresAuthRepository) DeleteAllUserRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	query := `DELETE FROM refresh_tokens WHERE user_id = $1`

	_, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("error eliminando refresh tokens del usuario: %w", err)
	}

	return nil
}

// RevokeToken inserta un JTI en la tabla de tokens revocados
func (r *PostgresAuthRepository) RevokeToken(ctx context.Context, jti uuid.UUID, userID uuid.UUID, expiresAt time.Time) error {
	query := `
		INSERT INTO revoked_tokens (jti, user_id, expires_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (jti) DO NOTHING`

	_, err := r.db.ExecContext(ctx, query, jti, userID, expiresAt)
	if err != nil {
		return fmt.Errorf("error revocando token: %w", err)
	}
	return nil
}

// IsTokenRevoked verifica si un JTI está en la lista de revocación
func (r *PostgresAuthRepository) IsTokenRevoked(ctx context.Context, jti uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM revoked_tokens WHERE jti = $1)`

	var exists bool
	err := r.db.QueryRowContext(ctx, query, jti).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("error verificando token revocado: %w", err)
	}
	return exists, nil
}

// RevokeAllUserTokens inserta una entrada genérica para revocar todos los tokens de un usuario
func (r *PostgresAuthRepository) RevokeAllUserTokens(ctx context.Context, userID uuid.UUID, expiresAt time.Time) error {
	jti := uuid.New()
	query := `
		INSERT INTO revoked_tokens (jti, user_id, expires_at)
		VALUES ($1, $2, $3)`

	_, err := r.db.ExecContext(ctx, query, jti, userID, expiresAt)
	if err != nil {
		return fmt.Errorf("error revocando todos los tokens del usuario: %w", err)
	}
	return nil
}

// CleanupExpiredRevocations elimina entradas de revocación expiradas
func (r *PostgresAuthRepository) CleanupExpiredRevocations(ctx context.Context) (int64, error) {
	query := `DELETE FROM revoked_tokens WHERE expires_at < NOW()`

	result, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("error limpiando tokens revocados expirados: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("error obteniendo filas afectadas: %w", err)
	}
	return count, nil
}

// GetUserByFederatedID obtiene un usuario por su ID federado
func (r *PostgresAuthRepository) GetUserByFederatedID(ctx context.Context, provider value_object.AuthProvider, federatedID string, tenantID *uuid.UUID) (port.UserData, error) {
	query := `
		SELECT id, email, password_hash, tenant_id, role_id, status, provider, federated_id
		FROM users 
		WHERE provider = $1 AND federated_id = $2`

	args := []interface{}{provider, federatedID}

	if tenantID != nil {
		query += ` AND tenant_id = $3`
		args = append(args, *tenantID)
	}

	var user port.UserData

	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.TenantID,
		&user.RoleID,
		&user.Status,
		&user.Provider,
		&user.FederatedID,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return user, fmt.Errorf("usuario no encontrado")
		}
		return user, fmt.Errorf("error obteniendo usuario por ID federado: %w", err)
	}

	return user, nil
}

// LinkFederatedID vincula un ID federado a un usuario existente. ACC-E02 T5:
// este UPDATE se ejecuta post-auth bajo account_app con RLS, nunca bajo
// iam_login (T1-D1). Si el contexto no trae tenant_id, falla closed.
func (r *PostgresAuthRepository) LinkFederatedID(ctx context.Context, userID uuid.UUID, provider value_object.AuthProvider, federatedID string) error {
	if tenantID, ok := sharedctx.TenantIDFromContext(ctx); ok {
		return sharedpostgres.WithRLSInTransaction(ctx, r.db, tenantID, func(ctx context.Context, tx *sql.Tx) error {
			return r.linkFederatedID(ctx, tx, userID, provider, federatedID)
		})
	}
	return r.linkFederatedID(ctx, r.db, userID, provider, federatedID)
}

func (r *PostgresAuthRepository) linkFederatedID(ctx context.Context, db execer, userID uuid.UUID, provider value_object.AuthProvider, federatedID string) error {
	query := `
		UPDATE users
		SET provider = $1, federated_id = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3`

	_, err := db.ExecContext(ctx, query, provider, federatedID, userID)
	if err != nil {
		return fmt.Errorf("error vinculando ID federado: %w", err)
	}

	return nil
}
