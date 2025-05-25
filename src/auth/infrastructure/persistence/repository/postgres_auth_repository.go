package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"

	"iam/src/auth/domain/entity"
	"iam/src/auth/domain/port"
	"iam/src/auth/domain/value_object"
)

type PostgresAuthRepository struct {
	db *sql.DB
}

func NewPostgresAuthRepository(db *sql.DB) port.AuthRepository {
	return &PostgresAuthRepository{
		db: db,
	}
}

// CreateRefreshToken almacena un nuevo refresh token
func (r *PostgresAuthRepository) CreateRefreshToken(ctx context.Context, token *entity.RefreshToken) error {
	query := `
		INSERT INTO refresh_tokens (id, user_id, token, expires_at, created_at) 
		VALUES ($1, $2, $3, $4, $5)`

	_, err := r.db.ExecContext(ctx, query,
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

// LinkFederatedID vincula un ID federado a un usuario existente
func (r *PostgresAuthRepository) LinkFederatedID(ctx context.Context, userID uuid.UUID, provider value_object.AuthProvider, federatedID string) error {
	query := `
		UPDATE users 
		SET provider = $1, federated_id = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3`

	_, err := r.db.ExecContext(ctx, query, provider, federatedID, userID)
	if err != nil {
		return fmt.Errorf("error vinculando ID federado: %w", err)
	}

	return nil
}
