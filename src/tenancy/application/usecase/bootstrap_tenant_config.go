package usecase

import (
	"context"
	"sort"

	"github.com/google/uuid"

	"github.com/hornosg/iam-service/src/tenancy/domain/entity"
	"github.com/hornosg/iam-service/src/tenancy/domain/port"
)

// DefaultConfigs define las configuraciones por defecto para un tenant nuevo
var DefaultConfigs = map[string]string{
	"catalog.stock_policy": "REQUIRE_STOCK",
}

// BootstrapTenantConfigUseCase inicializa la configuración default de un tenant.
// Es idempotente: sólo crea las configs que no existen y nunca sobrescribe una existente.
//
// ADR-005 §Bootstrap atómico: en tenant-service era GetByKey + Save, y Save es un upsert
// que en la carrera con un admin podía pisar su valor. Acá usa la operación insert-only
// del repositorio (ON CONFLICT DO NOTHING, una TX para todo el lote, sin pre-read).
type BootstrapTenantConfigUseCase struct {
	repository port.TenantConfigRepository
}

// NewBootstrapTenantConfigUseCase crea una nueva instancia del caso de uso
func NewBootstrapTenantConfigUseCase(repo port.TenantConfigRepository) *BootstrapTenantConfigUseCase {
	return &BootstrapTenantConfigUseCase{repository: repo}
}

// Execute retorna (configs_creadas, error). Ante error, configs_creadas es 0: el lote se revierte.
func (uc *BootstrapTenantConfigUseCase) Execute(ctx context.Context, tenantID uuid.UUID) (int, error) {
	keys := make([]string, 0, len(DefaultConfigs))
	for key := range DefaultConfigs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	configs := make([]*entity.TenantConfig, 0, len(keys))
	for _, key := range keys {
		configs = append(configs, entity.NewTenantConfig(tenantID, key, DefaultConfigs[key]))
	}

	createdCount, err := uc.repository.InsertMissing(ctx, tenantID, configs)
	if err != nil {
		return 0, err
	}
	return createdCount, nil
}
