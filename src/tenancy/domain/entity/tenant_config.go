package entity

import (
	"time"

	"github.com/google/uuid"
)

// TenantConfig representa una configuración key-value de un tenant.
// Mudada desde tenant-service (ACC-E04 T3); el agregado no cambió.
type TenantConfig struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	Key       string
	Value     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewTenantConfig crea una nueva instancia de TenantConfig
func NewTenantConfig(tenantID uuid.UUID, key, value string) *TenantConfig {
	now := time.Now()
	return &TenantConfig{
		ID:        uuid.New(),
		TenantID:  tenantID,
		Key:       key,
		Value:     value,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Update actualiza el valor de la configuración
func (tc *TenantConfig) Update(value string) {
	tc.Value = value
	tc.UpdatedAt = time.Now()
}
