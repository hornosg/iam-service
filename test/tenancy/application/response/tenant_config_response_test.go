package response_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/hornosg/iam-service/src/tenancy/application/response"
	"github.com/hornosg/iam-service/src/tenancy/domain/entity"
)

// Mudados de tenant-service (ACC-E04 T3, src/tenant/application/response/response_test.go).
// En iam los constructores se llaman TenantConfigFromEntity / NewSimpleConfigResponse
// (FromEntity / NewSimpleResponse en el origen); los casos son los mismos.

func TestTenantConfigFromEntity_MapsAllFields(t *testing.T) {
	cfg := entity.NewTenantConfig(uuid.New(), "catalog.stock_policy", "REQUIRE_STOCK")
	r := response.TenantConfigFromEntity(cfg)

	assert.Equal(t, cfg.ID, r.ID)
	assert.Equal(t, cfg.TenantID, r.TenantID)
	assert.Equal(t, cfg.Key, r.Key)
	assert.Equal(t, cfg.Value, r.Value)
	assert.Equal(t, cfg.CreatedAt, r.CreatedAt)
	assert.Equal(t, cfg.UpdatedAt, r.UpdatedAt)
}

func TestNewSimpleConfigResponse_WithValue(t *testing.T) {
	v := "REQUIRE_STOCK"
	r := response.NewSimpleConfigResponse("catalog.stock_policy", &v)

	assert.Equal(t, "catalog.stock_policy", r.Key)
	assert.NotNil(t, r.Value)
	assert.Equal(t, "REQUIRE_STOCK", *r.Value)
}

func TestNewSimpleConfigResponse_NilValue(t *testing.T) {
	r := response.NewSimpleConfigResponse("catalog.stock_policy", nil)

	assert.Equal(t, "catalog.stock_policy", r.Key)
	assert.Nil(t, r.Value)
}
