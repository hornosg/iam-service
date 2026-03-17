package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"iam/src/plan/domain/entity"
	"iam/src/plan/domain/exception"
	"iam/src/plan/domain/port"
	"iam/src/plan/domain/value_object"
	"github.com/mercadocercano/criteria"
)

type PostgresPlanRepository struct {
	db        *sql.DB
	converter *criteria.SQLCriteriaConverter
}

func NewPostgresPlanRepository(db *sql.DB) port.PlanCriteriaRepository {
	return &PostgresPlanRepository{
		db:        db,
		converter: criteria.NewSQLCriteriaConverter(),
	}
}

// Create inserta un nuevo plan en la base de datos
func (r *PostgresPlanRepository) Create(ctx context.Context, plan *entity.Plan) error {
	query := `
		INSERT INTO plans (id, name, description, type, status, max_users, price_month, price_year, features, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`

	_, err := r.db.ExecContext(ctx, query,
		plan.ID,
		plan.Name,
		plan.Description,
		plan.Type.String(),
		plan.Status.String(),
		plan.MaxUsers,
		plan.PriceMonth,
		plan.PriceYear,
		pq.Array(plan.Features),
		plan.CreatedAt,
		plan.UpdatedAt,
	)

	if err != nil {
		// Verificar si es error de constraint de nombre único
		if pqErr, ok := err.(*pq.Error); ok {
			if pqErr.Code == "23505" && pqErr.Constraint == "plans_name_key" {
				return exception.ErrPlanAlreadyExists
			}
		}
		return fmt.Errorf("error creating plan: %w", err)
	}

	return nil
}

// GetByID obtiene un plan por su ID
func (r *PostgresPlanRepository) GetByID(ctx context.Context, id uuid.UUID) (*entity.Plan, error) {
	query := `
		SELECT id, name, description, type, status, max_users, price_month, price_year, features, created_at, updated_at
		FROM plans
		WHERE id = $1
	`

	row := r.db.QueryRowContext(ctx, query, id)
	return r.scanPlan(row)
}

// GetByName obtiene un plan por su nombre
func (r *PostgresPlanRepository) GetByName(ctx context.Context, name string) (*entity.Plan, error) {
	query := `
		SELECT id, name, description, type, status, max_users, price_month, price_year, features, created_at, updated_at
		FROM plans
		WHERE name = $1
	`

	row := r.db.QueryRowContext(ctx, query, name)
	return r.scanPlan(row)
}

// Update actualiza un plan existente
func (r *PostgresPlanRepository) Update(ctx context.Context, plan *entity.Plan) error {
	query := `
		UPDATE plans
		SET name = $2, description = $3, type = $4, status = $5, max_users = $6, 
		    price_month = $7, price_year = $8, features = $9, updated_at = $10
		WHERE id = $1
	`

	result, err := r.db.ExecContext(ctx, query,
		plan.ID,
		plan.Name,
		plan.Description,
		plan.Type.String(),
		plan.Status.String(),
		plan.MaxUsers,
		plan.PriceMonth,
		plan.PriceYear,
		pq.Array(plan.Features),
		plan.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("error updating plan: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error checking rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return exception.ErrPlanNotFound
	}

	return nil
}

// Delete elimina un plan (soft delete cambiando status a DEPRECATED)
func (r *PostgresPlanRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE plans
		SET status = $2, updated_at = NOW()
		WHERE id = $1 AND status != $2
	`

	result, err := r.db.ExecContext(ctx, query, id, value_object.PlanStatusDeprecated.String())
	if err != nil {
		return fmt.Errorf("error deleting plan: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error checking rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return exception.ErrPlanNotFound
	}

	return nil
}

// GetByType obtiene planes por tipo
func (r *PostgresPlanRepository) GetByType(ctx context.Context, planType value_object.PlanType) ([]*entity.Plan, error) {
	query := `
		SELECT id, name, description, type, status, max_users, price_month, price_year, features, created_at, updated_at
		FROM plans
		WHERE type = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, planType.String())
	if err != nil {
		return nil, fmt.Errorf("error querying plans by type: %w", err)
	}
	defer rows.Close()

	return r.scanPlans(rows)
}

// GetByStatus obtiene planes por status
func (r *PostgresPlanRepository) GetByStatus(ctx context.Context, status value_object.PlanStatus) ([]*entity.Plan, error) {
	query := `
		SELECT id, name, description, type, status, max_users, price_month, price_year, features, created_at, updated_at
		FROM plans
		WHERE status = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, status.String())
	if err != nil {
		return nil, fmt.Errorf("error querying plans by status: %w", err)
	}
	defer rows.Close()

	return r.scanPlans(rows)
}

// GetActive obtiene solo los planes activos
func (r *PostgresPlanRepository) GetActive(ctx context.Context) ([]*entity.Plan, error) {
	return r.GetByStatus(ctx, value_object.PlanStatusActive)
}

// List obtiene planes con paginación
func (r *PostgresPlanRepository) List(ctx context.Context, limit, offset int) ([]*entity.Plan, error) {
	query := `
		SELECT id, name, description, type, status, max_users, price_month, price_year, features, created_at, updated_at
		FROM plans
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("error querying plans list: %w", err)
	}
	defer rows.Close()

	return r.scanPlans(rows)
}

// ExistsByName verifica si existe un plan con el nombre dado
func (r *PostgresPlanRepository) ExistsByName(ctx context.Context, name string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM plans WHERE name = $1)`

	var exists bool
	err := r.db.QueryRowContext(ctx, query, name).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("error checking if plan exists: %w", err)
	}

	return exists, nil
}

// Count cuenta el total de planes
func (r *PostgresPlanRepository) Count(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM plans`

	var count int
	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("error counting plans: %w", err)
	}

	return count, nil
}

// CountByStatus cuenta planes por status
func (r *PostgresPlanRepository) CountByStatus(ctx context.Context, status value_object.PlanStatus) (int, error) {
	query := `SELECT COUNT(*) FROM plans WHERE status = $1`

	var count int
	err := r.db.QueryRowContext(ctx, query, status.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("error counting plans by status: %w", err)
	}

	return count, nil
}

// scanPlan mapea una fila de la base de datos a una entidad Plan
func (r *PostgresPlanRepository) scanPlan(row *sql.Row) (*entity.Plan, error) {
	var typeStr, statusStr string
	var featuresArray pq.StringArray
	plan := &entity.Plan{}

	err := row.Scan(
		&plan.ID,
		&plan.Name,
		&plan.Description,
		&typeStr,
		&statusStr,
		&plan.MaxUsers,
		&plan.PriceMonth,
		&plan.PriceYear,
		&featuresArray,
		&plan.CreatedAt,
		&plan.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, exception.ErrPlanNotFound
		}
		return nil, fmt.Errorf("error scanning plan: %w", err)
	}

	// Construir value objects
	planType, err := value_object.NewPlanTypeFromString(typeStr)
	if err != nil {
		return nil, fmt.Errorf("invalid plan type in database: %w", err)
	}
	plan.Type = planType

	status, err := value_object.NewPlanStatusFromString(statusStr)
	if err != nil {
		return nil, fmt.Errorf("invalid plan status in database: %w", err)
	}
	plan.Status = status

	// Convertir features array
	plan.Features = []string(featuresArray)

	return plan, nil
}

// scanPlans mapea múltiples filas a entidades Plan
func (r *PostgresPlanRepository) scanPlans(rows *sql.Rows) ([]*entity.Plan, error) {
	var plans []*entity.Plan

	for rows.Next() {
		var typeStr, statusStr string
		var featuresArray pq.StringArray
		plan := &entity.Plan{}

		err := rows.Scan(
			&plan.ID,
			&plan.Name,
			&plan.Description,
			&typeStr,
			&statusStr,
			&plan.MaxUsers,
			&plan.PriceMonth,
			&plan.PriceYear,
			&featuresArray,
			&plan.CreatedAt,
			&plan.UpdatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("error scanning plan row: %w", err)
		}

		// Construir value objects
		planType, err := value_object.NewPlanTypeFromString(typeStr)
		if err != nil {
			return nil, fmt.Errorf("invalid plan type in database: %w", err)
		}
		plan.Type = planType

		status, err := value_object.NewPlanStatusFromString(statusStr)
		if err != nil {
			return nil, fmt.Errorf("invalid plan status in database: %w", err)
		}
		plan.Status = status

		// Convertir features array
		plan.Features = []string(featuresArray)

		plans = append(plans, plan)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating plan rows: %w", err)
	}

	return plans, nil
}

// SearchByCriteria busca planes usando criterios
func (r *PostgresPlanRepository) SearchByCriteria(ctx context.Context, crit criteria.Criteria) ([]*entity.Plan, error) {
	baseQuery := `
		SELECT id, name, description, type, status, max_users, price_month, price_year, features, created_at, updated_at
		FROM plans
	`

	query, params, err := r.converter.ToSelectSQL(baseQuery, crit)
	if err != nil {
		return nil, fmt.Errorf("invalid criteria: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, fmt.Errorf("error querying plans by criteria: %w", err)
	}
	defer rows.Close()

	return r.scanPlans(rows)
}

// CountByCriteria cuenta planes usando criterios
func (r *PostgresPlanRepository) CountByCriteria(ctx context.Context, crit criteria.Criteria) (int, error) {
	baseCountQuery := "SELECT COUNT(*) FROM plans"

	query, params, err := r.converter.ToCountSQL(baseCountQuery, crit)
	if err != nil {
		return 0, fmt.Errorf("invalid criteria: %w", err)
	}

	var count int
	err = r.db.QueryRowContext(ctx, query, params...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("error counting plans by criteria: %w", err)
	}

	return count, nil
}
