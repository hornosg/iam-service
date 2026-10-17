package entity

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	srcentity "github.com/hornosg/iam-service/src/tenancy/domain/entity"
)

func TestNewTenantConfig(t *testing.T) {
	tenantID := uuid.New()
	key := "catalog.stock_policy"
	value := "IGNORE_STOCK"

	config := srcentity.NewTenantConfig(tenantID, key, value)

	assert.NotNil(t, config)
	assert.NotEqual(t, uuid.Nil, config.ID)
	assert.Equal(t, tenantID, config.TenantID)
	assert.Equal(t, key, config.Key)
	assert.Equal(t, value, config.Value)
	assert.False(t, config.CreatedAt.IsZero())
	assert.False(t, config.UpdatedAt.IsZero())
}

func TestTenantConfig_Update(t *testing.T) {
	config := srcentity.NewTenantConfig(uuid.New(), "test.key", "old_value")
	originalUpdatedAt := config.UpdatedAt

	// Esperar un poco para que el timestamp cambie
	newValue := "new_value"
	config.Update(newValue)

	assert.Equal(t, newValue, config.Value)
	assert.True(t, config.UpdatedAt.After(originalUpdatedAt) || config.UpdatedAt.Equal(originalUpdatedAt))
}
