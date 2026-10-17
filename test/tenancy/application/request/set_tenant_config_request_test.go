package request_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hornosg/iam-service/src/tenancy/application/request"
)

// Mudados de tenant-service (ACC-E04 T3, src/tenant/application/request/request_test.go):
// la mudanza los había dejado atrás. Sólo cambia el import; los casos son los del origen.

func TestSetTenantConfigRequest_Valid(t *testing.T) {
	r := &request.SetTenantConfigRequest{Key: "catalog.stock_policy", Value: "REQUIRE_STOCK"}
	assert.NoError(t, r.Validate())
}

func TestSetTenantConfigRequest_EmptyKey(t *testing.T) {
	r := &request.SetTenantConfigRequest{Key: "", Value: "REQUIRE_STOCK"}
	err := r.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "key")
}

func TestSetTenantConfigRequest_EmptyValue(t *testing.T) {
	r := &request.SetTenantConfigRequest{Key: "catalog.stock_policy", Value: ""}
	err := r.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "value")
}

func TestValidationError_Error(t *testing.T) {
	e := &request.ValidationError{Field: "key", Message: "key cannot be empty"}
	assert.Equal(t, "key cannot be empty", e.Error())
}
