package port

import (
	"context"

	"github.com/google/uuid"

	"github.com/hornosg/iam-service/src/tenancy/domain/entity"
)

// TenantConfigRepository define el contrato para el repositorio de configuraciones
type TenantConfigRepository interface {
	// GetByKey obtiene una configuración por tenant y clave
	// Retorna (config, existe, error)
	GetByKey(ctx context.Context, tenantID uuid.UUID, key string) (*entity.TenantConfig, bool, error)

	// Save guarda o actualiza una configuración (upsert administrativo)
	Save(ctx context.Context, config *entity.TenantConfig) error

	// InsertMissing inserta en UNA transacción las configs que todavía no existen para el
	// tenant y nunca modifica las existentes (insert-only, ADR-005 §Bootstrap atómico).
	// Retorna la cantidad de filas realmente insertadas, publicada sólo tras commit;
	// ante cualquier error el lote se revierte y retorna 0.
	InsertMissing(ctx context.Context, tenantID uuid.UUID, configs []*entity.TenantConfig) (int, error)
}
