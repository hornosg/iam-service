package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/mercadocercano/criteria"
	"iam/src/user/domain/entity"
	"iam/src/user/domain/exception"
	"iam/src/user/domain/port"
	"iam/src/user/domain/value_object"
)

type PostgresUserRepository struct {
	db        *sql.DB
	converter *criteria.SQLCriteriaConverter
}

func NewPostgresUserRepository(db *sql.DB) port.UserCriteriaRepository {
	return &PostgresUserRepository{
		db:        db,
		converter: criteria.NewSQLCriteriaConverter(),
	}
}

// Create inserta un nuevo usuario en la base de datos
func (r *PostgresUserRepository) Create(ctx context.Context, user *entity.User) error {
	query := `
		INSERT INTO users (id, email, password_hash, tenant_id, role_id, status, provider, federated_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err := r.db.ExecContext(ctx, query,
		user.ID,
		user.Email.Value(),
		user.PasswordHash,
		user.TenantID,
		user.RoleID,
		user.Status.String(),
		user.Provider,
		user.FederatedID,
		user.CreatedAt,
		user.UpdatedAt,
	)

	if err != nil {
		// Verificar si es error de constraint de email único
		if pqErr, ok := err.(*pq.Error); ok {
			if pqErr.Code == "23505" && pqErr.Constraint == "users_email_tenant_unique" {
				return exception.ErrUserAlreadyExists
			}
		}
		return fmt.Errorf("error creating user: %w", err)
	}

	return nil
}

// GetByID obtiene un usuario por su ID
func (r *PostgresUserRepository) GetByID(ctx context.Context, id uuid.UUID) (*entity.User, error) {
	query := `
		SELECT id, email, password_hash, tenant_id, role_id, status, provider, federated_id, created_at, updated_at
		FROM users
		WHERE id = $1
	`

	row := r.db.QueryRowContext(ctx, query, id)
	return r.scanUser(row)
}

// GetByEmail obtiene un usuario por email y tenant
func (r *PostgresUserRepository) GetByEmail(ctx context.Context, email string, tenantID *uuid.UUID) (*entity.User, error) {
	var query string
	var args []interface{}

	if tenantID != nil {
		query = `
			SELECT id, email, password_hash, tenant_id, role_id, status, provider, federated_id, created_at, updated_at
			FROM users
			WHERE email = $1 AND tenant_id = $2
		`
		args = []interface{}{email, *tenantID}
	} else {
		query = `
			SELECT id, email, password_hash, tenant_id, role_id, status, provider, federated_id, created_at, updated_at
			FROM users
			WHERE email = $1
		`
		args = []interface{}{email}
	}

	row := r.db.QueryRowContext(ctx, query, args...)
	return r.scanUser(row)
}

// Update actualiza un usuario existente
func (r *PostgresUserRepository) Update(ctx context.Context, user *entity.User) error {
	query := `
		UPDATE users
		SET email = $2, password_hash = $3, tenant_id = $4, role_id = $5, status = $6, provider = $7, federated_id = $8, updated_at = $9
		WHERE id = $1
	`

	result, err := r.db.ExecContext(ctx, query,
		user.ID,
		user.Email.Value(),
		user.PasswordHash,
		user.TenantID,
		user.RoleID,
		user.Status.String(),
		user.Provider,
		user.FederatedID,
		time.Now(),
	)

	if err != nil {
		return fmt.Errorf("error updating user: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error checking rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return exception.ErrUserNotFound
	}

	return nil
}

// Delete elimina un usuario (soft delete cambiando status)
func (r *PostgresUserRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE users
		SET status = $2, updated_at = $3
		WHERE id = $1 AND status != $2
	`

	result, err := r.db.ExecContext(ctx, query, id, value_object.StatusDeleted.String(), time.Now())
	if err != nil {
		return fmt.Errorf("error deleting user: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error checking rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return exception.ErrUserNotFound
	}

	return nil
}

// GetByTenant obtiene usuarios de un tenant con paginación
func (r *PostgresUserRepository) GetByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]*entity.User, error) {
	query := `
		SELECT id, email, password_hash, tenant_id, role_id, status, provider, federated_id, created_at, updated_at
		FROM users
		WHERE tenant_id = $1 AND status != $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := r.db.QueryContext(ctx, query, tenantID, value_object.StatusDeleted.String(), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("error querying users by tenant: %w", err)
	}
	defer rows.Close()

	return r.scanUsers(rows)
}

// GetByStatus obtiene usuarios por status con paginación
func (r *PostgresUserRepository) GetByStatus(ctx context.Context, status value_object.UserStatus, limit, offset int) ([]*entity.User, error) {
	query := `
		SELECT id, email, password_hash, tenant_id, role_id, status, provider, federated_id, created_at, updated_at
		FROM users
		WHERE status = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, status.String(), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("error querying users by status: %w", err)
	}
	defer rows.Close()

	return r.scanUsers(rows)
}

// GetByRole obtiene usuarios por rol con paginación
func (r *PostgresUserRepository) GetByRole(ctx context.Context, roleID uuid.UUID, limit, offset int) ([]*entity.User, error) {
	query := `
		SELECT id, email, password_hash, tenant_id, role_id, status, provider, federated_id, created_at, updated_at
		FROM users
		WHERE role_id = $1 AND status != $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := r.db.QueryContext(ctx, query, roleID, value_object.StatusDeleted.String(), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("error querying users by role: %w", err)
	}
	defer rows.Close()

	return r.scanUsers(rows)
}

// ExistsByEmail verifica si existe un usuario con el email dado
func (r *PostgresUserRepository) ExistsByEmail(ctx context.Context, email string, tenantID *uuid.UUID) (bool, error) {
	var query string
	var args []interface{}

	if tenantID != nil {
		query = `SELECT EXISTS(SELECT 1 FROM users WHERE email = $1 AND tenant_id = $2 AND status != $3)`
		args = []interface{}{email, *tenantID, value_object.StatusDeleted.String()}
	} else {
		query = `SELECT EXISTS(SELECT 1 FROM users WHERE email = $1 AND status != $2)`
		args = []interface{}{email, value_object.StatusDeleted.String()}
	}

	var exists bool
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("error checking if user exists: %w", err)
	}

	return exists, nil
}

// CountByTenant cuenta usuarios de un tenant
func (r *PostgresUserRepository) CountByTenant(ctx context.Context, tenantID uuid.UUID) (int, error) {
	query := `SELECT COUNT(*) FROM users WHERE tenant_id = $1 AND status != $2`

	var count int
	err := r.db.QueryRowContext(ctx, query, tenantID, value_object.StatusDeleted.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("error counting users by tenant: %w", err)
	}

	return count, nil
}

// CountByStatus cuenta usuarios por status
func (r *PostgresUserRepository) CountByStatus(ctx context.Context, status value_object.UserStatus) (int, error) {
	query := `SELECT COUNT(*) FROM users WHERE status = $1`

	var count int
	err := r.db.QueryRowContext(ctx, query, status.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("error counting users by status: %w", err)
	}

	return count, nil
}

// scanUser mapea una fila de la base de datos a una entidad User
func (r *PostgresUserRepository) scanUser(row *sql.Row) (*entity.User, error) {
	var emailStr, statusStr string
	var federatedID sql.NullString
	user := &entity.User{}

	err := row.Scan(
		&user.ID,
		&emailStr,
		&user.PasswordHash,
		&user.TenantID,
		&user.RoleID,
		&statusStr,
		&user.Provider,
		&federatedID,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, exception.ErrUserNotFound
		}
		return nil, fmt.Errorf("error scanning user: %w", err)
	}

	// Asignar FederatedID manejando NULL
	if federatedID.Valid {
		user.FederatedID = federatedID.String
	} else {
		user.FederatedID = ""
	}

	// Construir value objects
	email, err := value_object.NewEmail(emailStr)
	if err != nil {
		return nil, fmt.Errorf("invalid email in database: %w", err)
	}
	user.Email = email

	status, err := value_object.NewUserStatusFromString(statusStr)
	if err != nil {
		return nil, fmt.Errorf("invalid status in database: %w", err)
	}
	user.Status = status

	return user, nil
}

// scanUsers mapea múltiples filas a entidades User
func (r *PostgresUserRepository) scanUsers(rows *sql.Rows) ([]*entity.User, error) {
	var users []*entity.User

	for rows.Next() {
		var emailStr, statusStr string
		var federatedID sql.NullString
		user := &entity.User{}

		err := rows.Scan(
			&user.ID,
			&emailStr,
			&user.PasswordHash,
			&user.TenantID,
			&user.RoleID,
			&statusStr,
			&user.Provider,
			&federatedID,
			&user.CreatedAt,
			&user.UpdatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("error scanning user row: %w", err)
		}

		// Asignar FederatedID manejando NULL
		if federatedID.Valid {
			user.FederatedID = federatedID.String
		} else {
			user.FederatedID = ""
		}

		// Construir value objects
		email, err := value_object.NewEmail(emailStr)
		if err != nil {
			return nil, fmt.Errorf("invalid email in database: %w", err)
		}
		user.Email = email

		status, err := value_object.NewUserStatusFromString(statusStr)
		if err != nil {
			return nil, fmt.Errorf("invalid status in database: %w", err)
		}
		user.Status = status

		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating user rows: %w", err)
	}

	return users, nil
}

// SearchByCriteria implementa la búsqueda usando criteria
func (r *PostgresUserRepository) SearchByCriteria(ctx context.Context, crit criteria.Criteria) ([]*entity.User, error) {
	baseQuery := `
		SELECT id, email, password_hash, tenant_id, role_id, status, provider, federated_id, created_at, updated_at
		FROM users
	`

	query, params, err := r.converter.ToSelectSQL(baseQuery, crit)
	if err != nil {
		return nil, fmt.Errorf("invalid criteria: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, fmt.Errorf("error executing search query: %w", err)
	}
	defer rows.Close()

	return r.scanUsers(rows)
}

// CountByCriteria implementa el conteo usando criteria
func (r *PostgresUserRepository) CountByCriteria(ctx context.Context, crit criteria.Criteria) (int, error) {
	baseCountQuery := "SELECT COUNT(*) FROM users"

	query, params, err := r.converter.ToCountSQL(baseCountQuery, crit)
	if err != nil {
		return 0, fmt.Errorf("invalid criteria: %w", err)
	}

	var count int
	err = r.db.QueryRowContext(ctx, query, params...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("error executing count query: %w", err)
	}

	return count, nil
}
