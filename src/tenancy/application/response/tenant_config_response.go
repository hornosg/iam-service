package response

import (
	"time"

	"github.com/google/uuid"

	"github.com/hornosg/iam-service/src/tenancy/domain/entity"
)

// TenantConfigResponse representa la respuesta de una configuración
type TenantConfigResponse struct {
	ID        uuid.UUID `json:"id"`
	TenantID  uuid.UUID `json:"tenant_id"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TenantConfigFromEntity crea una respuesta desde una entidad
func TenantConfigFromEntity(config *entity.TenantConfig) *TenantConfigResponse {
	return &TenantConfigResponse{
		ID:        config.ID,
		TenantID:  config.TenantID,
		Key:       config.Key,
		Value:     config.Value,
		CreatedAt: config.CreatedAt,
		UpdatedAt: config.UpdatedAt,
	}
}

// SimpleConfigResponse representa una respuesta simplificada (solo key y value)
type SimpleConfigResponse struct {
	Key   string  `json:"key"`
	Value *string `json:"value"` // Nullable para indicar que no existe
}

// NewSimpleConfigResponse crea una respuesta simplificada
func NewSimpleConfigResponse(key string, value *string) *SimpleConfigResponse {
	return &SimpleConfigResponse{
		Key:   key,
		Value: value,
	}
}
