package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/mercadocercano/criteria"
	"iam/src/role/domain/entity"
	"iam/src/role/domain/exception"
	"iam/src/role/domain/port"
	"iam/src/role/domain/value_object"
)

type PostgresRoleRepository struct {
	db        *sql.DB
	converter *criteria.SQLCriteriaConverter
}

func NewPostgresRoleRepository(db *sql.DB) port.RoleCriteriaRepository {
	return &PostgresRoleRepository{
		db:        db,
		converter: criteria.NewSQLCriteriaConverter(),
	}
}

// Create inserta un nuevo rol en la base de datos
func (r *PostgresRoleRepository) Create(ctx context.Context, role *entity.Role) error {
	query := `
		INSERT INTO roles (id, name, description, type, tenant_id, permissions, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := r.db.ExecContext(ctx, query,
		role.ID,
		role.Name,
		role.Description,
		role.Type.String(),
		role.TenantID,
		pq.Array(role.Permissions),
		role.IsActive,
		role.CreatedAt,
		role.UpdatedAt,
	)

	if err != nil {
		// Verificar si es error de constraint de nombre único por tenant
		if pqErr, ok := err.(*pq.Error); ok {
			if pqErr.Code == "23505" && pqErr.Constraint == "roles_name_tenant_unique" {
				return exception.ErrRoleAlreadyExists
			}
		}
		return fmt.Errorf("error creating role: %w", err)
	}

	return nil
}

// GetByID obtiene un rol por su ID
func (r *PostgresRoleRepository) GetByID(ctx context.Context, id uuid.UUID) (*entity.Role, error) {
	query := `
		SELECT id, name, description, type, tenant_id, permissions, is_active, created_at, updated_at
		FROM roles
		WHERE id = $1
	`

	row := r.db.QueryRowContext(ctx, query, id)
	return r.scanRole(row)
}

// GetByName obtiene un rol por nombre y tenant
func (r *PostgresRoleRepository) GetByName(ctx context.Context, name string, tenantID *uuid.UUID) (*entity.Role, error) {
	query := `
		SELECT id, name, description, type, tenant_id, permissions, is_active, created_at, updated_at
		FROM roles
		WHERE name = $1 AND (tenant_id = $2 OR ($2 IS NULL AND tenant_id IS NULL))
	`

	row := r.db.QueryRowContext(ctx, query, name, tenantID)
	return r.scanRole(row)
}

// Update actualiza un rol existente
func (r *PostgresRoleRepository) Update(ctx context.Context, role *entity.Role) error {
	query := `
		UPDATE roles
		SET name = $2, description = $3, type = $4, tenant_id = $5, permissions = $6, 
		    is_active = $7, updated_at = $8
		WHERE id = $1
	`

	result, err := r.db.ExecContext(ctx, query,
		role.ID,
		role.Name,
		role.Description,
		role.Type.String(),
		role.TenantID,
		pq.Array(role.Permissions),
		role.IsActive,
		role.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("error updating role: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error checking rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return exception.ErrRoleNotFound
	}

	return nil
}

// Delete elimina un rol (actualiza is_active a false)
func (r *PostgresRoleRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE roles
		SET is_active = false, updated_at = NOW()
		WHERE id = $1 AND is_active = true
	`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("error deleting role: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error checking rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return exception.ErrRoleNotFound
	}

	return nil
}

// GetByType obtiene roles por tipo
func (r *PostgresRoleRepository) GetByType(ctx context.Context, roleType value_object.RoleType) ([]*entity.Role, error) {
	query := `
		SELECT id, name, description, type, tenant_id, permissions, is_active, created_at, updated_at
		FROM roles
		WHERE type = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, roleType.String())
	if err != nil {
		return nil, fmt.Errorf("error querying roles by type: %w", err)
	}
	defer rows.Close()

	return r.scanRoles(rows)
}

// GetByTenant obtiene roles por tenant con paginación
func (r *PostgresRoleRepository) GetByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]*entity.Role, error) {
	query := `
		SELECT id, name, description, type, tenant_id, permissions, is_active, created_at, updated_at
		FROM roles
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("error querying roles by tenant: %w", err)
	}
	defer rows.Close()

	return r.scanRoles(rows)
}

// GetSystemRoles obtiene roles de sistema
func (r *PostgresRoleRepository) GetSystemRoles(ctx context.Context) ([]*entity.Role, error) {
	query := `
		SELECT id, name, description, type, tenant_id, permissions, is_active, created_at, updated_at
		FROM roles
		WHERE tenant_id IS NULL
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("error querying system roles: %w", err)
	}
	defer rows.Close()

	return r.scanRoles(rows)
}

// GetActiveRoles obtiene roles activos con filtro opcional por tenant
func (r *PostgresRoleRepository) GetActiveRoles(ctx context.Context, tenantID *uuid.UUID, limit, offset int) ([]*entity.Role, error) {
	var query string
	var args []interface{}

	if tenantID != nil {
		query = `
			SELECT id, name, description, type, tenant_id, permissions, is_active, created_at, updated_at
			FROM roles
			WHERE is_active = true AND tenant_id = $1
			ORDER BY created_at DESC
		`
		args = []interface{}{*tenantID}
	} else {
		query = `
			SELECT id, name, description, type, tenant_id, permissions, is_active, created_at, updated_at
			FROM roles
			WHERE is_active = true
			ORDER BY created_at DESC
		`
	}

	// Agregar paginación si se especifica
	if limit > 0 {
		if tenantID != nil {
			query += " LIMIT $2 OFFSET $3"
			args = append(args, limit, offset)
		} else {
			query += " LIMIT $1 OFFSET $2"
			args = append(args, limit, offset)
		}
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("error querying active roles: %w", err)
	}
	defer rows.Close()

	return r.scanRoles(rows)
}

// List obtiene roles con paginación
func (r *PostgresRoleRepository) List(ctx context.Context, limit, offset int) ([]*entity.Role, error) {
	query := `
		SELECT id, name, description, type, tenant_id, permissions, is_active, created_at, updated_at
		FROM roles
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("error querying roles list: %w", err)
	}
	defer rows.Close()

	return r.scanRoles(rows)
}

// ExistsByName verifica si existe un rol con el nombre dado en el tenant especificado
func (r *PostgresRoleRepository) ExistsByName(ctx context.Context, name string, tenantID *uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM roles WHERE name = $1 AND (tenant_id = $2 OR ($2 IS NULL AND tenant_id IS NULL)))`

	var exists bool
	err := r.db.QueryRowContext(ctx, query, name, tenantID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("error checking if role exists: %w", err)
	}

	return exists, nil
}

// Count cuenta el total de roles
func (r *PostgresRoleRepository) Count(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM roles`

	var count int
	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("error counting roles: %w", err)
	}

	return count, nil
}

// CountByTenant cuenta roles por tenant
func (r *PostgresRoleRepository) CountByTenant(ctx context.Context, tenantID uuid.UUID) (int, error) {
	query := `SELECT COUNT(*) FROM roles WHERE tenant_id = $1`

	var count int
	err := r.db.QueryRowContext(ctx, query, tenantID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("error counting roles by tenant: %w", err)
	}

	return count, nil
}

// CountByType cuenta roles por tipo
func (r *PostgresRoleRepository) CountByType(ctx context.Context, roleType value_object.RoleType) (int, error) {
	query := `SELECT COUNT(*) FROM roles WHERE type = $1`

	var count int
	err := r.db.QueryRowContext(ctx, query, roleType.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("error counting roles by type: %w", err)
	}

	return count, nil
}

// scanRole mapea una fila de la base de datos a una entidad Role
func (r *PostgresRoleRepository) scanRole(row *sql.Row) (*entity.Role, error) {
	var typeStr string
	var permissionsArray pq.StringArray
	role := &entity.Role{}

	err := row.Scan(
		&role.ID,
		&role.Name,
		&role.Description,
		&typeStr,
		&role.TenantID,
		&permissionsArray,
		&role.IsActive,
		&role.CreatedAt,
		&role.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, exception.ErrRoleNotFound
		}
		return nil, fmt.Errorf("error scanning role: %w", err)
	}

	// Construir value object
	roleType, err := value_object.NewRoleTypeFromString(typeStr)
	if err != nil {
		return nil, fmt.Errorf("invalid role type in database: %w", err)
	}
	role.Type = roleType

	// Convertir permissions array
	role.Permissions = []string(permissionsArray)

	return role, nil
}

// scanRoles mapea múltiples filas a entidades Role
func (r *PostgresRoleRepository) scanRoles(rows *sql.Rows) ([]*entity.Role, error) {
	roles := make([]*entity.Role, 0)

	for rows.Next() {
		var typeStr string
		var permissionsArray pq.StringArray
		role := &entity.Role{}

		err := rows.Scan(
			&role.ID,
			&role.Name,
			&role.Description,
			&typeStr,
			&role.TenantID,
			&permissionsArray,
			&role.IsActive,
			&role.CreatedAt,
			&role.UpdatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("error scanning role row: %w", err)
		}

		// Construir value object
		roleType, err := value_object.NewRoleTypeFromString(typeStr)
		if err != nil {
			return nil, fmt.Errorf("invalid role type in database: %w", err)
		}
		role.Type = roleType

		// Convertir permissions array
		role.Permissions = []string(permissionsArray)

		roles = append(roles, role)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating role rows: %w", err)
	}

	return roles, nil
}

// SearchByCriteria busca roles usando criterios
func (r *PostgresRoleRepository) SearchByCriteria(ctx context.Context, crit criteria.Criteria) ([]*entity.Role, error) {
	baseQuery := `
		SELECT id, name, description, type, tenant_id, permissions, is_active, created_at, updated_at
		FROM roles
	`

	query, params, err := r.converter.ToSelectSQL(baseQuery, crit)
	if err != nil {
		return nil, fmt.Errorf("invalid criteria: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, fmt.Errorf("error querying roles by criteria: %w", err)
	}
	defer rows.Close()

	return r.scanRoles(rows)
}

// CountByCriteria cuenta roles usando criterios
func (r *PostgresRoleRepository) CountByCriteria(ctx context.Context, crit criteria.Criteria) (int, error) {
	baseCountQuery := "SELECT COUNT(*) FROM roles"

	query, params, err := r.converter.ToCountSQL(baseCountQuery, crit)
	if err != nil {
		return 0, fmt.Errorf("invalid criteria: %w", err)
	}

	var count int
	err = r.db.QueryRowContext(ctx, query, params...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("error counting roles by criteria: %w", err)
	}

	return count, nil
}
