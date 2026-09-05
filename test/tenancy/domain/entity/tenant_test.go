package entity

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	srcEntity "github.com/hornosg/iam-service/src/tenancy/domain/entity"
	"github.com/hornosg/iam-service/src/tenancy/domain/value_object"
)

func TestNewTenant_WithValidParams_CreatesWithDefaults(t *testing.T) {
	ownerID := uuid.New()

	tenant := srcEntity.NewTenant("Mi Tienda", "mi-tienda", "Descripcion", value_object.TenantTypeStartup, ownerID)

	assert.NotEqual(t, uuid.Nil, tenant.ID)
	assert.Equal(t, "Mi Tienda", tenant.Name)
	assert.Equal(t, "mi-tienda", tenant.Slug)
	assert.Equal(t, value_object.TenantTypeStartup, tenant.Type)
	assert.Equal(t, value_object.TenantStatusActive, tenant.Status)
	assert.Equal(t, 10, tenant.MaxUsers)
	assert.Equal(t, 0, tenant.UserCount)
	assert.Equal(t, ownerID, tenant.OwnerID)
	assert.NotNil(t, tenant.Features)
	assert.Nil(t, tenant.PlanID)
}

func TestTenant_ChangeStatus_ChangesStatusAndUpdatesTimestamp(t *testing.T) {
	mother := Create()
	tenant := mother.WithDefaults()

	tenant.ChangeStatus(value_object.TenantStatusSuspended)

	assert.Equal(t, value_object.TenantStatusSuspended, tenant.Status)
}

func TestTenant_Activate_SetsStatusActive(t *testing.T) {
	mother := Create()
	tenant := mother.Suspended()

	tenant.Activate()

	assert.Equal(t, value_object.TenantStatusActive, tenant.Status)
}

func TestTenant_Suspend_SetsStatusSuspended(t *testing.T) {
	mother := Create()
	tenant := mother.WithDefaults()

	tenant.Suspend()

	assert.Equal(t, value_object.TenantStatusSuspended, tenant.Status)
}

func TestTenant_Delete_SetsStatusDeleted(t *testing.T) {
	mother := Create()
	tenant := mother.WithDefaults()

	tenant.Delete()

	assert.Equal(t, value_object.TenantStatusDeleted, tenant.Status)
}

func TestTenant_SetPlan_SetsPlanIDAndSubscribedAt(t *testing.T) {
	mother := Create()
	tenant := mother.WithDefaults()
	planID := uuid.New()

	tenant.SetPlan(planID)

	require.NotNil(t, tenant.PlanID)
	assert.Equal(t, planID, *tenant.PlanID)
	assert.NotNil(t, tenant.SubscribedAt)
	assert.True(t, tenant.HasPlan())
}

func TestTenant_RemovePlan_ClearsPlanFields(t *testing.T) {
	mother := Create()
	planID := uuid.New()
	tenant := mother.WithPlan(planID)

	tenant.RemovePlan()

	assert.Nil(t, tenant.PlanID)
	assert.Nil(t, tenant.SubscribedAt)
	assert.False(t, tenant.HasPlan())
}

func TestTenant_UpdateUserLimits_ChangesMaxUsers(t *testing.T) {
	mother := Create()
	tenant := mother.WithDefaults()

	tenant.UpdateUserLimits(50)

	assert.Equal(t, 50, tenant.MaxUsers)
}

func TestTenant_IncrementUserCount_WithCapacity_Succeeds(t *testing.T) {
	mother := Create()
	tenant := mother.WithUserLimits(10, 5)

	err := tenant.IncrementUserCount()

	require.NoError(t, err)
	assert.Equal(t, 6, tenant.UserCount)
}

func TestTenant_IncrementUserCount_AtLimit_ReturnsError(t *testing.T) {
	mother := Create()
	tenant := mother.WithUserLimits(5, 5)

	err := tenant.IncrementUserCount()

	assert.Error(t, err)
	assert.Equal(t, 5, tenant.UserCount)
}

func TestTenant_IncrementUserCount_UnlimitedUsers_AlwaysSucceeds(t *testing.T) {
	mother := Create()
	tenant := mother.Enterprise()

	err := tenant.IncrementUserCount()

	require.NoError(t, err)
	assert.Equal(t, 1, tenant.UserCount)
}

func TestTenant_DecrementUserCount_WithUsers_Decrements(t *testing.T) {
	mother := Create()
	tenant := mother.WithUserLimits(10, 5)

	tenant.DecrementUserCount()

	assert.Equal(t, 4, tenant.UserCount)
}

func TestTenant_DecrementUserCount_AtZero_StaysAtZero(t *testing.T) {
	mother := Create()
	tenant := mother.WithDefaults()

	tenant.DecrementUserCount()

	assert.Equal(t, 0, tenant.UserCount)
}

func TestTenant_Features_EnableAndDisable(t *testing.T) {
	mother := Create()
	tenant := mother.WithDefaults()

	assert.False(t, tenant.HasFriendsFamily())
	assert.False(t, tenant.HasPremiumAnalytics())

	tenant.EnableFriendsFamily()
	assert.True(t, tenant.HasFriendsFamily())

	tenant.DisableFriendsFamily()
	assert.False(t, tenant.HasFriendsFamily())

	tenant.EnablePremiumAnalytics()
	assert.True(t, tenant.HasPremiumAnalytics())

	tenant.DisablePremiumAnalytics()
	assert.False(t, tenant.HasPremiumAnalytics())
}

func TestTenant_IsActive_ActiveTenant_ReturnsTrue(t *testing.T) {
	mother := Create()
	tenant := mother.WithDefaults()

	assert.True(t, tenant.IsActive())
}

func TestTenant_IsActive_SuspendedTenant_ReturnsFalse(t *testing.T) {
	mother := Create()
	tenant := mother.Suspended()

	assert.False(t, tenant.IsActive())
}

func TestTenant_CanAccess_ActiveAndNotExpired_ReturnsTrue(t *testing.T) {
	mother := Create()
	tenant := mother.WithDefaults()

	assert.True(t, tenant.CanAccess())
}

func TestTenant_CanAccess_ExpiredTenant_ReturnsFalse(t *testing.T) {
	mother := Create()
	tenant := mother.Expired()

	assert.False(t, tenant.CanAccess())
}

func TestTenant_CanAccess_SuspendedTenant_ReturnsFalse(t *testing.T) {
	mother := Create()
	tenant := mother.Suspended()

	assert.False(t, tenant.CanAccess())
}

func TestTenant_IsExpired_WithFutureExpiration_ReturnsFalse(t *testing.T) {
	mother := Create()
	future := time.Now().AddDate(0, 0, 30)
	tenant := mother.WithExpiration(future)

	assert.False(t, tenant.IsExpired())
}

func TestTenant_IsExpired_WithPastExpiration_ReturnsTrue(t *testing.T) {
	mother := Create()
	tenant := mother.Expired()

	assert.True(t, tenant.IsExpired())
}

func TestTenant_IsExpired_WithNoExpiration_ReturnsFalse(t *testing.T) {
	mother := Create()
	tenant := mother.WithDefaults()

	assert.False(t, tenant.IsExpired())
}

func TestTenant_CanAddUser_ActiveWithCapacity_ReturnsTrue(t *testing.T) {
	mother := Create()
	tenant := mother.WithUserLimits(10, 5)

	assert.True(t, tenant.CanAddUser())
}

func TestTenant_CanAddUser_AtLimit_ReturnsFalse(t *testing.T) {
	mother := Create()
	tenant := mother.WithUserLimits(5, 5)

	assert.False(t, tenant.CanAddUser())
}

func TestTenant_CanAddUser_UnlimitedUsers_ReturnsTrue(t *testing.T) {
	mother := Create()
	tenant := mother.Enterprise()

	assert.True(t, tenant.CanAddUser())
}

func TestTenant_CanBeModified_ActiveTenant_ReturnsTrue(t *testing.T) {
	mother := Create()
	tenant := mother.WithDefaults()

	assert.True(t, tenant.CanBeModified())
}

func TestTenant_CanBeModified_DeletedTenant_ReturnsFalse(t *testing.T) {
	mother := Create()
	tenant := mother.Deleted()

	assert.False(t, tenant.CanBeModified())
}

func TestTenant_HasCustomDomain_WithDomain_ReturnsTrue(t *testing.T) {
	mother := Create()
	tenant := mother.WithDomain("custom.domain.com")

	assert.True(t, tenant.HasCustomDomain())
}

func TestTenant_HasCustomDomain_WithoutDomain_ReturnsFalse(t *testing.T) {
	mother := Create()
	tenant := mother.WithDefaults()

	assert.False(t, tenant.HasCustomDomain())
}

func TestTenant_Settings_SetAndGet(t *testing.T) {
	mother := Create()
	tenant := mother.WithDefaults()

	tenant.UpdateSetting("theme", "dark")

	val, exists := tenant.GetSetting("theme")
	assert.True(t, exists)
	assert.Equal(t, "dark", val)
}

func TestTenant_Settings_GetNonExistent_ReturnsFalse(t *testing.T) {
	mother := Create()
	tenant := mother.WithDefaults()

	_, exists := tenant.GetSetting("nonexistent")

	assert.False(t, exists)
}
