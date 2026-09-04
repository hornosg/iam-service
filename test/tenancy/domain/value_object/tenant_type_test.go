package value_object_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"iam/src/tenancy/domain/value_object"
)

func TestTenantType_AllTypes_AreValid(t *testing.T) {
	types := []value_object.TenantType{
		value_object.TenantTypePersonal,
		value_object.TenantTypeStartup,
		value_object.TenantTypeBusiness,
		value_object.TenantTypeEnterprise,
	}

	for _, tt := range types {
		t.Run(string(tt), func(t *testing.T) {
			assert.True(t, tt.IsValid())
		})
	}
}

func TestTenantType_InvalidType_IsNotValid(t *testing.T) {
	invalidTypes := []value_object.TenantType{
		"UNKNOWN",
		"personal",
		"",
	}

	for _, tt := range invalidTypes {
		t.Run(string(tt), func(t *testing.T) {
			assert.False(t, tt.IsValid())
		})
	}
}

func TestNewTenantTypeFromString_ValidString_ReturnsType(t *testing.T) {
	tests := []struct {
		input    string
		expected value_object.TenantType
	}{
		{"PERSONAL", value_object.TenantTypePersonal},
		{"STARTUP", value_object.TenantTypeStartup},
		{"BUSINESS", value_object.TenantTypeBusiness},
		{"ENTERPRISE", value_object.TenantTypeEnterprise},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := value_object.NewTenantTypeFromString(tt.input)

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewTenantTypeFromString_InvalidString_ReturnsError(t *testing.T) {
	result, err := value_object.NewTenantTypeFromString("INVALID")

	assert.Error(t, err)
	assert.Equal(t, value_object.TenantType(""), result)
}

func TestTenantType_GetDefaultUserLimit_ReturnsCorrectLimits(t *testing.T) {
	tests := []struct {
		tenantType value_object.TenantType
		expected   int
	}{
		{value_object.TenantTypePersonal, 1},
		{value_object.TenantTypeStartup, 10},
		{value_object.TenantTypeBusiness, 100},
		{value_object.TenantTypeEnterprise, -1}, // Unlimited
	}

	for _, tt := range tests {
		t.Run(string(tt.tenantType), func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.tenantType.GetDefaultUserLimit())
		})
	}
}

func TestTenantType_RequiresApproval_OnlyEnterprise(t *testing.T) {
	assert.False(t, value_object.TenantTypePersonal.RequiresApproval())
	assert.False(t, value_object.TenantTypeStartup.RequiresApproval())
	assert.False(t, value_object.TenantTypeBusiness.RequiresApproval())
	assert.True(t, value_object.TenantTypeEnterprise.RequiresApproval())
}

func TestTenantType_String_ReturnsStringRepresentation(t *testing.T) {
	assert.Equal(t, "PERSONAL", value_object.TenantTypePersonal.String())
	assert.Equal(t, "ENTERPRISE", value_object.TenantTypeEnterprise.String())
}
