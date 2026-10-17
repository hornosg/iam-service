package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	sharedpostgres "github.com/hornosg/iam-service/src/shared/postgres"
	"github.com/hornosg/iam-service/src/tenancy/domain/entity"
	"github.com/hornosg/iam-service/src/tenancy/domain/port"
)

// ErrTenantConfigTenantMismatch se devuelve si una config del lote no es del tenant de la TX.
var ErrTenantConfigTenantMismatch = errors.New("tenant config: tenant_id del lote distinto al tenant de la transacción")

// BeforeCommitHook corre dentro de la TX, después del SQL y antes del commit. Si devuelve
// error, la TX hace rollback. Existe para que los tests de integración retengan una TX
// abierta o fuercen un rollback sobre el camino real; producción no lo usa.
type BeforeCommitHook func(ctx context.Context) error

// PostgresTenantConfigRepository implementa el repositorio sobre `tenant_config` en iam_db.
//
// RLS (migración 025, ACC-E04 T2): `tenant_config` tiene ROW LEVEL SECURITY forzado con la
// policy `tenant_isolation`. Cada operación corre dentro de WithRLSInTransaction, que fija
// `app.tenant_id` con SET LOCAL — sin él, el rol NOBYPASSRLS `account_app` no ve filas
// (fail-closed). El filtro manual `WHERE tenant_id = $` se mantiene como defensa en profundidad.
type PostgresTenantConfigRepository struct {
	db           *sql.DB
	beforeCommit BeforeCommitHook
}

// NewPostgresTenantConfigRepository crea una nueva instancia del repositorio
func NewPostgresTenantConfigRepository(db *sql.DB) port.TenantConfigRepository {
	return &PostgresTenantConfigRepository{db: db}
}

// NewPostgresTenantConfigRepositoryWithHook crea el repositorio con un hook previo al commit (tests).
func NewPostgresTenantConfigRepositoryWithHook(db *sql.DB, beforeCommit BeforeCommitHook) port.TenantConfigRepository {
	return &PostgresTenantConfigRepository{db: db, beforeCommit: beforeCommit}
}

func (r *PostgresTenantConfigRepository) inTx(ctx context.Context, tenantID uuid.UUID, fn func(context.Context, *sql.Tx) error) error {
	return sharedpostgres.WithRLSInTransaction(ctx, r.db, tenantID, func(ctx context.Context, tx *sql.Tx) error {
		if err := fn(ctx, tx); err != nil {
			return err
		}
		if r.beforeCommit != nil {
			return r.beforeCommit(ctx)
		}
		return nil
	})
}

// GetByKey obtiene una configuración por tenant y clave. Tenant: tenantID param.
func (r *PostgresTenantConfigRepository) GetByKey(ctx context.Context, tenantID uuid.UUID, key string) (*entity.TenantConfig, bool, error) {
	query := `
		SELECT id, tenant_id, config_key, config_value, created_at, updated_at
		FROM tenant_config
		WHERE tenant_id = $1 AND config_key = $2
	`

	var config entity.TenantConfig
	var found bool
	err := r.inTx(ctx, tenantID, func(ctx context.Context, tx *sql.Tx) error {
		scanErr := tx.QueryRowContext(ctx, query, tenantID, key).Scan(
			&config.ID,
			&config.TenantID,
			&config.Key,
			&config.Value,
			&config.CreatedAt,
			&config.UpdatedAt,
		)
		if scanErr == sql.ErrNoRows {
			return nil
		}
		if scanErr != nil {
			return scanErr
		}
		found = true
		return nil
	})

	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}

	return &config, true, nil
}

// Save guarda o actualiza una configuración (upsert administrativo). Tenant: config.TenantID
// (path del WITH CHECK). NO lo usa el bootstrap: ver InsertMissing.
func (r *PostgresTenantConfigRepository) Save(ctx context.Context, config *entity.TenantConfig) error {
	query := `
		INSERT INTO tenant_config (id, tenant_id, config_key, config_value, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (tenant_id, config_key)
		DO UPDATE SET
			config_value = EXCLUDED.config_value,
			updated_at = EXCLUDED.updated_at
	`

	return r.inTx(ctx, config.TenantID, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query,
			config.ID,
			config.TenantID,
			config.Key,
			config.Value,
			config.CreatedAt,
			config.UpdatedAt,
		)
		return err
	})
}

// InsertMissing inserta el lote insert-only en una sola TX (ADR-005 §Bootstrap atómico).
// El índice único (tenant_id, config_key) resuelve la carrera aunque no exista fila que
// bloquear: en conflicto no se toca valor, ID ni timestamps. El contador suma RowsAffected
// y se publica sólo si el commit salió bien; cualquier error devuelve 0.
func (r *PostgresTenantConfigRepository) InsertMissing(ctx context.Context, tenantID uuid.UUID, configs []*entity.TenantConfig) (int, error) {
	query := `
		INSERT INTO tenant_config (id, tenant_id, config_key, config_value, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (tenant_id, config_key) DO NOTHING
	`

	for _, config := range configs {
		if config.TenantID != tenantID {
			return 0, ErrTenantConfigTenantMismatch
		}
	}

	inserted := 0
	err := r.inTx(ctx, tenantID, func(ctx context.Context, tx *sql.Tx) error {
		for _, config := range configs {
			res, err := tx.ExecContext(ctx, query,
				config.ID,
				config.TenantID,
				config.Key,
				config.Value,
				config.CreatedAt,
				config.UpdatedAt,
			)
			if err != nil {
				return err
			}
			n, err := res.RowsAffected()
			if err != nil {
				return err
			}
			inserted += int(n)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return inserted, nil
}
