package usecase

import (
	"context"

	"github.com/google/uuid"

	"github.com/hornosg/iam-service/src/tenancy/domain/entity"
	"github.com/hornosg/iam-service/src/tenancy/domain/port"
)

// GetTenantConfigUseCase obtiene una configuración por clave
type GetTenantConfigUseCase struct {
	repository port.TenantConfigRepository
}

// NewGetTenantConfigUseCase crea una nueva instancia del caso de uso
func NewGetTenantConfigUseCase(repo port.TenantConfigRepository) *GetTenantConfigUseCase {
	return &GetTenantConfigUseCase{repository: repo}
}

// Execute retorna (config, existe, error)
func (uc *GetTenantConfigUseCase) Execute(ctx context.Context, tenantID uuid.UUID, key string) (*entity.TenantConfig, bool, error) {
	return uc.repository.GetByKey(ctx, tenantID, key)
}
