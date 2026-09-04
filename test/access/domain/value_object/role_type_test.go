package value_object_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"iam/src/access/domain/value_object"
)

func TestRoleType_AllTypes_AreValid(t *testing.T) {
	types := []value_object.RoleType{
		value_object.RoleTypeSystemAdmin,
		value_object.RoleTypeTenantAdmin,
		value_object.RoleTypeUser,
		value_object.RoleTypeReadOnly,
		value_object.RoleTypeCustom,
	}

	for _, rt := range types {
		t.Run(string(rt), func(t *testing.T) {
			assert.True(t, rt.IsValid())
		})
	}
}

func TestRoleType_InvalidType_IsNotValid(t *testing.T) {
	invalidTypes := []value_object.RoleType{
		"UNKNOWN",
		"admin",
		"",
		"SUPER_ADMIN",
	}

	for _, rt := range invalidTypes {
		t.Run(string(rt), func(t *testing.T) {
			assert.False(t, rt.IsValid())
		})
	}
}

func TestNewRoleTypeFromString_ValidString_ReturnsType(t *testing.T) {
	tests := []struct {
		input    string
		expected value_object.RoleType
	}{
		{"SYSTEM_ADMIN", value_object.RoleTypeSystemAdmin},
		{"TENANT_ADMIN", value_object.RoleTypeTenantAdmin},
		{"USER", value_object.RoleTypeUser},
		{"READ_ONLY", value_object.RoleTypeReadOnly},
		{"CUSTOM", value_object.RoleTypeCustom},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := value_object.NewRoleTypeFromString(tt.input)

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewRoleTypeFromString_InvalidString_ReturnsError(t *testing.T) {
	result, err := value_object.NewRoleTypeFromString("INVALID")

	assert.Error(t, err)
	assert.Equal(t, value_object.RoleType(""), result)
}

func TestRoleType_IsSystemLevel_OnlySystemAdmin(t *testing.T) {
	assert.True(t, value_object.RoleTypeSystemAdmin.IsSystemLevel())
	assert.False(t, value_object.RoleTypeTenantAdmin.IsSystemLevel())
	assert.False(t, value_object.RoleTypeUser.IsSystemLevel())
	assert.False(t, value_object.RoleTypeReadOnly.IsSystemLevel())
	assert.False(t, value_object.RoleTypeCustom.IsSystemLevel())
}

func TestRoleType_IsTenantLevel_AllExceptSystemAdmin(t *testing.T) {
	assert.False(t, value_object.RoleTypeSystemAdmin.IsTenantLevel())
	assert.True(t, value_object.RoleTypeTenantAdmin.IsTenantLevel())
	assert.True(t, value_object.RoleTypeUser.IsTenantLevel())
	assert.True(t, value_object.RoleTypeReadOnly.IsTenantLevel())
	assert.True(t, value_object.RoleTypeCustom.IsTenantLevel())
}

func TestRoleType_CanManageUsers_OnlyAdmins(t *testing.T) {
	assert.True(t, value_object.RoleTypeSystemAdmin.CanManageUsers())
	assert.True(t, value_object.RoleTypeTenantAdmin.CanManageUsers())
	assert.False(t, value_object.RoleTypeUser.CanManageUsers())
	assert.False(t, value_object.RoleTypeReadOnly.CanManageUsers())
	assert.False(t, value_object.RoleTypeCustom.CanManageUsers())
}

func TestRoleType_CanManageTenant_OnlyAdmins(t *testing.T) {
	assert.True(t, value_object.RoleTypeSystemAdmin.CanManageTenant())
	assert.True(t, value_object.RoleTypeTenantAdmin.CanManageTenant())
	assert.False(t, value_object.RoleTypeUser.CanManageTenant())
	assert.False(t, value_object.RoleTypeReadOnly.CanManageTenant())
	assert.False(t, value_object.RoleTypeCustom.CanManageTenant())
}

func TestRoleType_String_ReturnsStringRepresentation(t *testing.T) {
	assert.Equal(t, "SYSTEM_ADMIN", value_object.RoleTypeSystemAdmin.String())
	assert.Equal(t, "CUSTOM", value_object.RoleTypeCustom.String())
}
