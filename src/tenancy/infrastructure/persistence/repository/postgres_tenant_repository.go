package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/hornosg/go-shared/criteria"
	sharedctx "github.com/hornosg/iam-service/src/shared/context"
	sharedpostgres "github.com/hornosg/iam-service/src/shared/postgres"
	"github.com/hornosg/iam-service/src/tenancy/domain/entity"
	"github.com/hornosg/iam-service/src/tenancy/domain/exception"
	"github.com/hornosg/iam-service/src/tenancy/domain/port"
	"github.com/hornosg/iam-service/src/tenancy/domain/value_object"
)

type PostgresTenantRepository struct {
	db        *sql.DB
	converter *criteria.SQLCriteriaConverter
}

func NewPostgresTenantRepository(db *sql.DB) port.TenantCriteriaRepository {
	return &PostgresTenantRepository{
		db:        db,
		converter: criteria.NewSQLCriteriaConverter(),
	}
}

// querier abstrae *sql.DB y *sql.Tx para que los métodos internos corran
// indistintamente dentro o fuera de la transacción RLS.
type querier interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

// withRLS corre fn dentro de una transacción que fija los GUC de sesión que el
// contexto justifica: app.tenant_id si porta un tenant_id, y app.is_system_admin
// si el gate de autorización (Authorize, T8d) verificó la autoridad system_admin.
// Sin ninguno de los dos, corre sobre el pool directo (fail-closed: contra la 019
// toda policy evalúa NULL → 0 filas). Patrón copiado de
// src/auth/infrastructure/persistence/repository/postgres_auth_repository.go
// (ACC-E02 T4/T5) y requerido por T8b para que las policies RLS de tenants vean
// el tenant de sesión.
func withRLS[T any](ctx context.Context, db *sql.DB, fn func(context.Context, querier) (T, error)) (T, error) {
	locals := map[string]string{}
	if tenantID, ok := sharedctx.TenantIDFromContext(ctx); ok {
		locals[sharedpostgres.TenantVar] = tenantID.String()
	}
	if sharedctx.IsSystemAdminFromContext(ctx) {
		locals[sharedpostgres.SystemAdminVar] = sharedpostgres.SystemAdminTrue
	}
	if len(locals) == 0 {
		return fn(ctx, db)
	}
	var result T
	err := sharedpostgres.WithSessionLocals(ctx, db, locals, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		result, err = fn(ctx, tx)
		return err
	})
	return result, err
}

// Create inserta un nuevo tenant en la base de datos.
//
// ACC-E02 T8d — por qué NO corre por withRLS ni fija app.is_system_admin:
// el caller (POST /tenants, provision) no tiene tenant de sesión y, por lo
// general, tampoco autoridad system_admin verificada (el gate de provision
// acepta el scope tenant:provision a secas). Fijar app.is_system_admin desde
// ese scope está PROHIBIDO por la condición vinculante del gate L4 de T8b
// (objeción (C)-1): ese GUC abre la tabla `tenants` entera y sólo puede
// derivarse de un scope `system:admin` verificado. La policy tenant_isolation
// (019) acepta el INSERT por la otra rama del WITH CHECK — id = app.tenant_id —
// así que esta transacción corre con app.tenant_id = id del tenant que nace:
// es la autoridad mínima para el caso de uso (la fila que se crea; el GUC
// muere con la tx por SET LOCAL) y un caller provision no gana por esto
// ninguna lectura ni escritura cross-tenant.
func (r *PostgresTenantRepository) Create(ctx context.Context, tenant *entity.Tenant) error {
	err := sharedpostgres.WithSessionLocals(ctx, r.db, map[string]string{
		sharedpostgres.TenantVar: tenant.ID.String(),
	}, func(ctx context.Context, tx *sql.Tx) error {
		query := `
			INSERT INTO tenants (
				id, name, slug, description, type, status, owner_id, domain,
				plan_id, user_count, max_users, settings, features, expires_at,
				created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		`

		settingsJSON, err := json.Marshal(tenant.Settings)
		if err != nil {
			return fmt.Errorf("error marshaling settings: %w", err)
		}

		featuresJSON, err := json.Marshal(tenant.GetFeatures())
		if err != nil {
			return fmt.Errorf("error marshaling features: %w", err)
		}

		domainVal := sql.NullString{String: tenant.Domain, Valid: tenant.Domain != ""}

		_, err = tx.ExecContext(ctx, query,
			tenant.ID,
			tenant.Name,
			tenant.Slug,
			tenant.Description,
			tenant.Type.String(),
			tenant.Status.String(),
			tenant.OwnerID,
			domainVal,
			tenant.PlanID,
			tenant.UserCount,
			tenant.MaxUsers,
			settingsJSON,
			featuresJSON,
			tenant.ExpiresAt,
			tenant.CreatedAt,
			tenant.UpdatedAt,
		)

		if err != nil {
			// Verificar errores de constraint únicos
			if pqErr, ok := err.(*pq.Error); ok {
				if pqErr.Code == "23505" {
					switch pqErr.Constraint {
					case "tenants_slug_unique":
						return exception.ErrSlugAlreadyExists
					case "tenants_domain_unique":
						return exception.ErrDomainAlreadyExists
					}
				}
			}
			return fmt.Errorf("error creating tenant: %w", err)
		}

		return nil
	})
	return err
}

// GetByID obtiene un tenant por su ID
func (r *PostgresTenantRepository) GetByID(ctx context.Context, id uuid.UUID) (*entity.Tenant, error) {
	return withRLS(ctx, r.db, func(ctx context.Context, q querier) (*entity.Tenant, error) {
		query := `
			SELECT id, name, slug, description, type, status, owner_id, domain,
			       plan_id, user_count, max_users, settings, features, expires_at,
			       created_at, updated_at
			FROM tenants
			WHERE id = $1
		`

		row := q.QueryRowContext(ctx, query, id)
		return r.scanTenant(row)
	})
}

// GetBySlug obtiene un tenant por su slug
func (r *PostgresTenantRepository) GetBySlug(ctx context.Context, slug string) (*entity.Tenant, error) {
	return withRLS(ctx, r.db, func(ctx context.Context, q querier) (*entity.Tenant, error) {
		query := `
			SELECT id, name, slug, description, type, status, owner_id, domain,
			       plan_id, user_count, max_users, settings, features, expires_at,
			       created_at, updated_at
			FROM tenants
			WHERE slug = $1
		`

		row := q.QueryRowContext(ctx, query, slug)
		return r.scanTenant(row)
	})
}

// GetByDomain obtiene un tenant por su dominio personalizado
func (r *PostgresTenantRepository) GetByDomain(ctx context.Context, domain string) (*entity.Tenant, error) {
	return withRLS(ctx, r.db, func(ctx context.Context, q querier) (*entity.Tenant, error) {
		query := `
			SELECT id, name, slug, description, type, status, owner_id, domain,
			       plan_id, user_count, max_users, settings, features, expires_at,
			       created_at, updated_at
			FROM tenants
			WHERE domain = $1
		`

		row := q.QueryRowContext(ctx, query, domain)
		return r.scanTenant(row)
	})
}

// Update actualiza un tenant existente
func (r *PostgresTenantRepository) Update(ctx context.Context, tenant *entity.Tenant) error {
	_, err := withRLS(ctx, r.db, func(ctx context.Context, q querier) (struct{}, error) {
		query := `
			UPDATE tenants
			SET name = $2, description = $3, type = $4, status = $5, domain = $6,
			    plan_id = $7, user_count = $8, max_users = $9, settings = $10,
			    features = $11, expires_at = $12, updated_at = $13
			WHERE id = $1
		`

		settingsJSON, err := json.Marshal(tenant.Settings)
		if err != nil {
			return struct{}{}, fmt.Errorf("error marshaling settings: %w", err)
		}

		featuresJSON, err := json.Marshal(tenant.GetFeatures())
		if err != nil {
			return struct{}{}, fmt.Errorf("error marshaling features: %w", err)
		}

		domainUpdateVal := sql.NullString{String: tenant.Domain, Valid: tenant.Domain != ""}

		result, err := q.ExecContext(ctx, query,
			tenant.ID,
			tenant.Name,
			tenant.Description,
			tenant.Type.String(),
			tenant.Status.String(),
			domainUpdateVal,
			tenant.PlanID,
			tenant.UserCount,
			tenant.MaxUsers,
			settingsJSON,
			featuresJSON,
			tenant.ExpiresAt,
			tenant.UpdatedAt,
		)

		if err != nil {
			// Verificar errores de constraint únicos
			if pqErr, ok := err.(*pq.Error); ok {
				if pqErr.Code == "23505" && pqErr.Constraint == "tenants_domain_unique" {
					return struct{}{}, exception.ErrDomainAlreadyExists
				}
			}
			return struct{}{}, fmt.Errorf("error updating tenant: %w", err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return struct{}{}, fmt.Errorf("error checking rows affected: %w", err)
		}

		if rowsAffected == 0 {
			return struct{}{}, exception.ErrTenantNotFound
		}

		return struct{}{}, nil
	})
	return err
}

// Delete elimina un tenant (actualiza status a DELETED)
func (r *PostgresTenantRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := withRLS(ctx, r.db, func(ctx context.Context, q querier) (struct{}, error) {
		query := `
			UPDATE tenants
			SET status = 'DELETED', updated_at = NOW()
			WHERE id = $1 AND status != 'DELETED'
		`

		result, err := q.ExecContext(ctx, query, id)
		if err != nil {
			return struct{}{}, fmt.Errorf("error deleting tenant: %w", err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return struct{}{}, fmt.Errorf("error checking rows affected: %w", err)
		}

		if rowsAffected == 0 {
			return struct{}{}, exception.ErrTenantNotFound
		}

		return struct{}{}, nil
	})
	return err
}

// List obtiene tenants con paginación
func (r *PostgresTenantRepository) List(ctx context.Context, limit, offset int) ([]*entity.Tenant, error) {
	return withRLS(ctx, r.db, func(ctx context.Context, q querier) ([]*entity.Tenant, error) {
		query := `
			SELECT id, name, slug, description, type, status, owner_id, domain,
			       plan_id, user_count, max_users, settings, features, expires_at,
			       created_at, updated_at
			FROM tenants
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2
		`

		rows, err := q.QueryContext(ctx, query, limit, offset)
		if err != nil {
			return nil, fmt.Errorf("error querying tenants: %w", err)
		}
		defer rows.Close()

		return r.scanTenants(rows)
	})
}

// GetByOwner obtiene tenants por owner
func (r *PostgresTenantRepository) GetByOwner(ctx context.Context, ownerID uuid.UUID) ([]*entity.Tenant, error) {
	return withRLS(ctx, r.db, func(ctx context.Context, q querier) ([]*entity.Tenant, error) {
		query := `
			SELECT id, name, slug, description, type, status, owner_id, domain,
			       plan_id, user_count, max_users, settings, features, expires_at,
			       created_at, updated_at
			FROM tenants
			WHERE owner_id = $1
			ORDER BY created_at DESC
		`

		rows, err := q.QueryContext(ctx, query, ownerID)
		if err != nil {
			return nil, fmt.Errorf("error querying tenants by owner: %w", err)
		}
		defer rows.Close()

		return r.scanTenants(rows)
	})
}

// GetByStatus obtiene tenants por status con paginación
func (r *PostgresTenantRepository) GetByStatus(ctx context.Context, status value_object.TenantStatus, limit, offset int) ([]*entity.Tenant, error) {
	return withRLS(ctx, r.db, func(ctx context.Context, q querier) ([]*entity.Tenant, error) {
		query := `
			SELECT id, name, slug, description, type, status, owner_id, domain,
			       plan_id, user_count, max_users, settings, features, expires_at,
			       created_at, updated_at
			FROM tenants
			WHERE status = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`

		rows, err := q.QueryContext(ctx, query, status.String(), limit, offset)
		if err != nil {
			return nil, fmt.Errorf("error querying tenants by status: %w", err)
		}
		defer rows.Close()

		return r.scanTenants(rows)
	})
}

// GetByType obtiene tenants por tipo con paginación
func (r *PostgresTenantRepository) GetByType(ctx context.Context, tenantType value_object.TenantType, limit, offset int) ([]*entity.Tenant, error) {
	return withRLS(ctx, r.db, func(ctx context.Context, q querier) ([]*entity.Tenant, error) {
		query := `
			SELECT id, name, slug, description, type, status, owner_id, domain,
			       plan_id, user_count, max_users, settings, features, expires_at,
			       created_at, updated_at
			FROM tenants
			WHERE type = $1
			ORDER BY created_at DESC
		`

		// Si limit es -1, no aplicamos LIMIT
		if limit > 0 {
			query += " LIMIT $2 OFFSET $3"
			rows, err := q.QueryContext(ctx, query, tenantType.String(), limit, offset)
			if err != nil {
				return nil, fmt.Errorf("error querying tenants by type: %w", err)
			}
			defer rows.Close()
			return r.scanTenants(rows)
		} else {
			rows, err := q.QueryContext(ctx, query, tenantType.String())
			if err != nil {
				return nil, fmt.Errorf("error querying tenants by type: %w", err)
			}
			defer rows.Close()
			return r.scanTenants(rows)
		}
	})
}

// GetByPlan obtiene tenants por plan
func (r *PostgresTenantRepository) GetByPlan(ctx context.Context, planID uuid.UUID, limit, offset int) ([]*entity.Tenant, error) {
	return withRLS(ctx, r.db, func(ctx context.Context, q querier) ([]*entity.Tenant, error) {
		query := `
			SELECT id, name, slug, description, type, status, owner_id, domain,
			       plan_id, user_count, max_users, settings, features, expires_at,
			       created_at, updated_at
			FROM tenants
			WHERE plan_id = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`

		rows, err := q.QueryContext(ctx, query, planID, limit, offset)
		if err != nil {
			return nil, fmt.Errorf("error querying tenants by plan: %w", err)
		}
		defer rows.Close()

		return r.scanTenants(rows)
	})
}

// GetActive obtiene tenants activos con paginación
func (r *PostgresTenantRepository) GetActive(ctx context.Context, limit, offset int) ([]*entity.Tenant, error) {
	return withRLS(ctx, r.db, func(ctx context.Context, q querier) ([]*entity.Tenant, error) {
		query := `
			SELECT id, name, slug, description, type, status, owner_id, domain,
			       plan_id, user_count, max_users, settings, features, expires_at,
			       created_at, updated_at
			FROM tenants
			WHERE status = 'ACTIVE'
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2
		`

		rows, err := q.QueryContext(ctx, query, limit, offset)
		if err != nil {
			return nil, fmt.Errorf("error querying active tenants: %w", err)
		}
		defer rows.Close()

		return r.scanTenants(rows)
	})
}

// GetExpiring obtiene tenants que expiran en los próximos días
func (r *PostgresTenantRepository) GetExpiring(ctx context.Context, days int) ([]*entity.Tenant, error) {
	return withRLS(ctx, r.db, func(ctx context.Context, q querier) ([]*entity.Tenant, error) {
		query := `
			SELECT id, name, slug, description, type, status, owner_id, domain,
			       plan_id, user_count, max_users, settings, features, expires_at,
			       created_at, updated_at
			FROM tenants
			WHERE expires_at IS NOT NULL
			  AND expires_at <= $1
			  AND status = 'ACTIVE'
			ORDER BY expires_at ASC
		`

		expirationDate := time.Now().AddDate(0, 0, days)
		rows, err := q.QueryContext(ctx, query, expirationDate)
		if err != nil {
			return nil, fmt.Errorf("error querying expiring tenants: %w", err)
		}
		defer rows.Close()

		return r.scanTenants(rows)
	})
}

// ExistsBySlug verifica si existe un tenant con el slug
func (r *PostgresTenantRepository) ExistsBySlug(ctx context.Context, slug string) (bool, error) {
	return withRLS(ctx, r.db, func(ctx context.Context, q querier) (bool, error) {
		query := `SELECT 1 FROM tenants WHERE slug = $1`
		var exists int
		err := q.QueryRowContext(ctx, query, slug).Scan(&exists)
		if err == sql.ErrNoRows {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("error checking slug existence: %w", err)
		}
		return true, nil
	})
}

// ExistsByDomain verifica si existe un tenant con el dominio
func (r *PostgresTenantRepository) ExistsByDomain(ctx context.Context, domain string) (bool, error) {
	return withRLS(ctx, r.db, func(ctx context.Context, q querier) (bool, error) {
		query := `SELECT 1 FROM tenants WHERE domain = $1`
		var exists int
		err := q.QueryRowContext(ctx, query, domain).Scan(&exists)
		if err == sql.ErrNoRows {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("error checking domain existence: %w", err)
		}
		return true, nil
	})
}

// Count obtiene el total de tenants
func (r *PostgresTenantRepository) Count(ctx context.Context) (int, error) {
	return withRLS(ctx, r.db, func(ctx context.Context, q querier) (int, error) {
		query := `SELECT COUNT(*) FROM tenants`
		var count int
		err := q.QueryRowContext(ctx, query).Scan(&count)
		if err != nil {
			return 0, fmt.Errorf("error counting tenants: %w", err)
		}
		return count, nil
	})
}

// CountByStatus obtiene el total de tenants por status
func (r *PostgresTenantRepository) CountByStatus(ctx context.Context, status value_object.TenantStatus) (int, error) {
	return withRLS(ctx, r.db, func(ctx context.Context, q querier) (int, error) {
		query := `SELECT COUNT(*) FROM tenants WHERE status = $1`
		var count int
		err := q.QueryRowContext(ctx, query, status.String()).Scan(&count)
		if err != nil {
			return 0, fmt.Errorf("error counting tenants by status: %w", err)
		}
		return count, nil
	})
}

// CountByOwner obtiene el total de tenants por owner
func (r *PostgresTenantRepository) CountByOwner(ctx context.Context, ownerID uuid.UUID) (int, error) {
	return withRLS(ctx, r.db, func(ctx context.Context, q querier) (int, error) {
		query := `SELECT COUNT(*) FROM tenants WHERE owner_id = $1`
		var count int
		err := q.QueryRowContext(ctx, query, ownerID).Scan(&count)
		if err != nil {
			return 0, fmt.Errorf("error counting tenants by owner: %w", err)
		}
		return count, nil
	})
}

// CountByPlan obtiene el total de tenants por plan
func (r *PostgresTenantRepository) CountByPlan(ctx context.Context, planID uuid.UUID) (int, error) {
	return withRLS(ctx, r.db, func(ctx context.Context, q querier) (int, error) {
		query := `SELECT COUNT(*) FROM tenants WHERE plan_id = $1`
		var count int
		err := q.QueryRowContext(ctx, query, planID).Scan(&count)
		if err != nil {
			return 0, fmt.Errorf("error counting tenants by plan: %w", err)
		}
		return count, nil
	})
}

// scanTenant escanea una fila en una entidad Tenant
func (r *PostgresTenantRepository) scanTenant(row *sql.Row) (*entity.Tenant, error) {
	var tenant entity.Tenant
	var tenantTypeStr, tenantStatusStr string
	var settingsJSON, featuresJSON []byte
	var domain sql.NullString
	var planID sql.NullString
	var expiresAt sql.NullTime

	err := row.Scan(
		&tenant.ID,
		&tenant.Name,
		&tenant.Slug,
		&tenant.Description,
		&tenantTypeStr,
		&tenantStatusStr,
		&tenant.OwnerID,
		&domain,
		&planID,
		&tenant.UserCount,
		&tenant.MaxUsers,
		&settingsJSON,
		&featuresJSON,
		&expiresAt,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, exception.ErrTenantNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("error scanning tenant: %w", err)
	}

	// Convertir tipo
	tenantType, err := value_object.NewTenantTypeFromString(tenantTypeStr)
	if err != nil {
		return nil, fmt.Errorf("error parsing tenant type: %w", err)
	}
	tenant.Type = tenantType

	// Convertir status
	tenantStatus, err := value_object.NewTenantStatusFromString(tenantStatusStr)
	if err != nil {
		return nil, fmt.Errorf("error parsing tenant status: %w", err)
	}
	tenant.Status = tenantStatus

	// Dominio opcional
	if domain.Valid {
		tenant.Domain = domain.String
	}

	// Plan ID opcional
	if planID.Valid {
		if parsedPlanID, parseErr := uuid.Parse(planID.String); parseErr == nil {
			tenant.PlanID = &parsedPlanID
		}
	}

	// Fecha de expiración opcional
	if expiresAt.Valid {
		tenant.ExpiresAt = &expiresAt.Time
	}

	// Deserializar settings
	if len(settingsJSON) > 0 {
		if err := json.Unmarshal(settingsJSON, &tenant.Settings); err != nil {
			return nil, fmt.Errorf("error unmarshaling settings: %w", err)
		}
	}

	// Deserializar features
	if len(featuresJSON) > 0 {
		if err := json.Unmarshal(featuresJSON, &tenant.Features); err != nil {
			return nil, fmt.Errorf("error unmarshaling features: %w", err)
		}
	}

	return &tenant, nil
}

// scanTenants escanea múltiples filas en entidades Tenant
func (r *PostgresTenantRepository) scanTenants(rows *sql.Rows) ([]*entity.Tenant, error) {
	tenants := make([]*entity.Tenant, 0)

	for rows.Next() {
		var tenant entity.Tenant
		var tenantTypeStr, tenantStatusStr string
		var settingsJSON, featuresJSON []byte
		var domain sql.NullString
		var planID sql.NullString
		var expiresAt sql.NullTime

		err := rows.Scan(
			&tenant.ID,
			&tenant.Name,
			&tenant.Slug,
			&tenant.Description,
			&tenantTypeStr,
			&tenantStatusStr,
			&tenant.OwnerID,
			&domain,
			&planID,
			&tenant.UserCount,
			&tenant.MaxUsers,
			&settingsJSON,
			&featuresJSON,
			&expiresAt,
			&tenant.CreatedAt,
			&tenant.UpdatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("error scanning tenant: %w", err)
		}

		// Convertir tipo
		tenantType, err := value_object.NewTenantTypeFromString(tenantTypeStr)
		if err != nil {
			return nil, fmt.Errorf("error parsing tenant type: %w", err)
		}
		tenant.Type = tenantType

		// Convertir status
		tenantStatus, err := value_object.NewTenantStatusFromString(tenantStatusStr)
		if err != nil {
			return nil, fmt.Errorf("error parsing tenant status: %w", err)
		}
		tenant.Status = tenantStatus

		// Dominio opcional
		if domain.Valid {
			tenant.Domain = domain.String
		}

		// Plan ID opcional
		if planID.Valid {
			if parsedPlanID, parseErr := uuid.Parse(planID.String); parseErr == nil {
				tenant.PlanID = &parsedPlanID
			}
		}

		// Fecha de expiración opcional
		if expiresAt.Valid {
			tenant.ExpiresAt = &expiresAt.Time
		}

		// Deserializar settings
		if len(settingsJSON) > 0 {
			if err := json.Unmarshal(settingsJSON, &tenant.Settings); err != nil {
				return nil, fmt.Errorf("error unmarshaling settings: %w", err)
			}
		}

		// Deserializar features
		if len(featuresJSON) > 0 {
			if err := json.Unmarshal(featuresJSON, &tenant.Features); err != nil {
				return nil, fmt.Errorf("error unmarshaling features: %w", err)
			}
		}

		tenants = append(tenants, &tenant)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tenant rows: %w", err)
	}

	return tenants, nil
}

// SearchByCriteria busca tenants usando criterios
func (r *PostgresTenantRepository) SearchByCriteria(ctx context.Context, crit criteria.Criteria) ([]*entity.Tenant, error) {
	return withRLS(ctx, r.db, func(ctx context.Context, q querier) ([]*entity.Tenant, error) {
		baseQuery := `
			SELECT id, name, slug, description, type, status, owner_id, domain,
			       plan_id, user_count, max_users, settings, features, expires_at,
			       created_at, updated_at
			FROM tenants
		`

		query, params, err := r.converter.ToSelectSQL(baseQuery, crit)
		if err != nil {
			return nil, fmt.Errorf("invalid criteria: %w", err)
		}

		rows, err := q.QueryContext(ctx, query, params...)
		if err != nil {
			return nil, fmt.Errorf("error querying tenants by criteria: %w", err)
		}
		defer rows.Close()

		return r.scanTenants(rows)
	})
}

// CountByCriteria cuenta tenants usando criterios
func (r *PostgresTenantRepository) CountByCriteria(ctx context.Context, crit criteria.Criteria) (int, error) {
	return withRLS(ctx, r.db, func(ctx context.Context, q querier) (int, error) {
		baseCountQuery := "SELECT COUNT(*) FROM tenants"

		query, params, err := r.converter.ToCountSQL(baseCountQuery, crit)
		if err != nil {
			return 0, fmt.Errorf("invalid criteria: %w", err)
		}

		var count int
		err = q.QueryRowContext(ctx, query, params...).Scan(&count)
		if err != nil {
			return 0, fmt.Errorf("error counting tenants by criteria: %w", err)
		}

		return count, nil
	})
}
