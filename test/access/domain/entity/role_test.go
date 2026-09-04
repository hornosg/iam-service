package entity

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	srcEntity "iam/src/access/domain/entity"
	"iam/src/access/domain/value_object"
)

func TestNewRole_WithValidParams_CreatesWithDefaults(t *testing.T) {
	role := srcEntity.NewRole("Admin", "Admin role", value_object.RoleTypeTenantAdmin)

	assert.NotEqual(t, uuid.Nil, role.ID)
	assert.Equal(t, "Admin", role.Name)
	assert.Equal(t, value_object.RoleTypeTenantAdmin, role.Type)
	assert.Empty(t, role.Permissions)
	assert.True(t, role.IsActive)
}

func TestNewRole_SystemRole_IsSystemRole(t *testing.T) {
	role := srcEntity.NewRole("System Admin", "Desc", value_object.RoleTypeSystemAdmin)

	assert.True(t, role.IsSystemRole())
	assert.False(t, role.IsTenantRole())
}

func TestRole_AddPermission_AddsUniquePermission(t *testing.T) {
	mother := Create()
	role := mother.WithDefaults()
	initialLen := len(role.Permissions)

	role.AddPermission("new:permission")

	assert.Len(t, role.Permissions, initialLen+1)
	assert.True(t, role.HasPermission("new:permission"))
}

func TestRole_AddPermission_DuplicatePermission_DoesNotAdd(t *testing.T) {
	mother := Create()
	role := mother.WithPermissions([]string{"read:basic"})

	role.AddPermission("read:basic")

	assert.Len(t, role.Permissions, 1)
}

func TestRole_RemovePermission_ExistingPermission_Removes(t *testing.T) {
	mother := Create()
	role := mother.WithPermissions([]string{"read:basic", "write:basic"})

	role.RemovePermission("read:basic")

	assert.False(t, role.HasPermission("read:basic"))
	assert.True(t, role.HasPermission("write:basic"))
	assert.Len(t, role.Permissions, 1)
}

func TestRole_RemovePermission_NonExistent_DoesNothing(t *testing.T) {
	mother := Create()
	role := mother.WithPermissions([]string{"read:basic"})

	role.RemovePermission("nonexistent")

	assert.Len(t, role.Permissions, 1)
}

func TestRole_HasPermission_ExistingPermission_ReturnsTrue(t *testing.T) {
	mother := Create()
	role := mother.WithPermissions([]string{"read:basic", "write:basic"})

	assert.True(t, role.HasPermission("read:basic"))
}

func TestRole_HasPermission_NonExistent_ReturnsFalse(t *testing.T) {
	mother := Create()
	role := mother.WithDefaults()

	assert.False(t, role.HasPermission("nonexistent"))
}

// ACC-E02 T11: el tipo de sistema se define por Type, no por tenant.
func TestRole_IsSystemRole_SystemAdmin_ReturnsTrue(t *testing.T) {
	mother := Create()
	role := mother.SystemAdmin()

	assert.True(t, role.IsSystemRole())
}

func TestRole_IsSystemRole_UserType_ReturnsFalse(t *testing.T) {
	mother := Create()
	role := mother.User()

	assert.False(t, role.IsSystemRole())
}

func TestRole_IsTenantRole_UserType_ReturnsTrue(t *testing.T) {
	mother := Create()
	role := mother.User()

	assert.True(t, role.IsTenantRole())
}

func TestRole_IsTenantRole_SystemAdmin_ReturnsFalse(t *testing.T) {
	mother := Create()
	role := mother.SystemAdmin()

	assert.False(t, role.IsTenantRole())
}

func TestRole_Activate_SetsIsActiveTrue(t *testing.T) {
	mother := Create()
	role := mother.Inactive()

	role.Activate()

	assert.True(t, role.IsActive)
}

func TestRole_Deactivate_SetsIsActiveFalse(t *testing.T) {
	mother := Create()
	role := mother.WithDefaults()

	role.Deactivate()

	assert.False(t, role.IsActive)
}

func TestRole_CanManageUsers_AdminRoles_ReturnsTrue(t *testing.T) {
	mother := Create()

	assert.True(t, mother.SystemAdmin().CanManageUsers())
	assert.True(t, mother.TenantAdmin().CanManageUsers())
}

func TestRole_CanManageUsers_NonAdminRoles_ReturnsFalse(t *testing.T) {
	mother := Create()

	assert.False(t, mother.User().CanManageUsers())
	assert.False(t, mother.ReadOnly().CanManageUsers())
	assert.False(t, mother.Custom().CanManageUsers())
}

func TestRole_CanManageTenant_AdminRoles_ReturnsTrue(t *testing.T) {
	mother := Create()

	assert.True(t, mother.SystemAdmin().CanManageTenant())
	assert.True(t, mother.TenantAdmin().CanManageTenant())
}

func TestRole_CanManageTenant_NonAdminRoles_ReturnsFalse(t *testing.T) {
	mother := Create()

	assert.False(t, mother.User().CanManageTenant())
	assert.False(t, mother.ReadOnly().CanManageTenant())
}

func TestRole_UpdateDetails_ChangesNameAndDescription(t *testing.T) {
	mother := Create()
	role := mother.WithDefaults()

	role.UpdateDetails("New Name", "New Description")

	assert.Equal(t, "New Name", role.Name)
	assert.Equal(t, "New Description", role.Description)
}
