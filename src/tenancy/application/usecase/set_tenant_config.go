package usecase

import (
	"context"

	"github.com/google/uuid"

	"github.com/hornosg/iam-service/src/tenancy/domain/entity"
	"github.com/hornosg/iam-service/src/tenancy/domain/port"
)

// SetTenantConfigUseCase establece una configuración (upsert administrativo)
type SetTenantConfigUseCase struct {
	repository port.TenantConfigRepository
}

// NewSetTenantConfigUseCase crea una nueva instancia del caso de uso
func NewSetTenantConfigUseCase(repo port.TenantConfigRepository) *SetTenantConfigUseCase {
	return &SetTenantConfigUseCase{repository: repo}
}

// Execute ejecuta el upsert y retorna la configuración guardada
func (uc *SetTenantConfigUseCase) Execute(ctx context.Context, tenantID uuid.UUID, key, value string) (*entity.TenantConfig, error) {
	// Buscar si ya existe
	existingConfig, exists, err := uc.repository.GetByKey(ctx, tenantID, key)
	if err != nil {
		return nil, err
	}

	var config *entity.TenantConfig

	if exists {
		// UPDATE: actualizar el valor existente
		existingConfig.Update(value)
		config = existingConfig
	} else {
		// INSERT: crear nueva configuración
		config = entity.NewTenantConfig(tenantID, key, value)
	}

	// Guardar (upsert)
	if err := uc.repository.Save(ctx, config); err != nil {
		return nil, err
	}

	return config, nil
}
