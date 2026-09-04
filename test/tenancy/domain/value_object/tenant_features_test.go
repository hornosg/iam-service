package value_object_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"iam/src/tenancy/domain/value_object"
)

func TestNewTenantFeatures_DefaultValues_AllDisabled(t *testing.T) {
	features := value_object.NewTenantFeatures()

	assert.False(t, features.FriendsFamily)
	assert.False(t, features.PremiumAnalytics)
	assert.False(t, features.HasFriendsFamily())
	assert.False(t, features.HasPremiumAnalytics())
}

func TestNewTenantFeaturesWithValues_SetsCorrectValues(t *testing.T) {
	features := value_object.NewTenantFeaturesWithValues(true, false)

	assert.True(t, features.FriendsFamily)
	assert.False(t, features.PremiumAnalytics)
}

func TestTenantFeatures_UpdateFriendsFamily_EnableAndDisable(t *testing.T) {
	features := value_object.NewTenantFeatures()

	features.UpdateFriendsFamily(true)
	assert.True(t, features.HasFriendsFamily())

	features.UpdateFriendsFamily(false)
	assert.False(t, features.HasFriendsFamily())
}

func TestTenantFeatures_UpdatePremiumAnalytics_EnableAndDisable(t *testing.T) {
	features := value_object.NewTenantFeatures()

	features.UpdatePremiumAnalytics(true)
	assert.True(t, features.HasPremiumAnalytics())

	features.UpdatePremiumAnalytics(false)
	assert.False(t, features.HasPremiumAnalytics())
}

func TestTenantFeatures_ToMap_ReturnsCorrectMap(t *testing.T) {
	features := value_object.NewTenantFeaturesWithValues(true, false)

	result := features.ToMap()

	assert.Equal(t, true, result["friends_family"])
	assert.Equal(t, false, result["premium_analytics"])
}

func TestTenantFeatures_ToMap_AllEnabled_ReturnsCorrectMap(t *testing.T) {
	features := value_object.NewTenantFeaturesWithValues(true, true)

	result := features.ToMap()

	assert.Equal(t, true, result["friends_family"])
	assert.Equal(t, true, result["premium_analytics"])
}

func TestTenantFeatures_FromMap_LoadsCorrectValues(t *testing.T) {
	features := value_object.NewTenantFeatures()
	data := map[string]interface{}{
		"friends_family":    true,
		"premium_analytics": true,
	}

	features.FromMap(data)

	assert.True(t, features.HasFriendsFamily())
	assert.True(t, features.HasPremiumAnalytics())
}

func TestTenantFeatures_FromMap_PartialData_OnlyUpdatesProvided(t *testing.T) {
	features := value_object.NewTenantFeatures()
	data := map[string]interface{}{
		"friends_family": true,
	}

	features.FromMap(data)

	assert.True(t, features.HasFriendsFamily())
	assert.False(t, features.HasPremiumAnalytics())
}

func TestTenantFeatures_FromMap_EmptyMap_KeepsDefaults(t *testing.T) {
	features := value_object.NewTenantFeatures()

	features.FromMap(map[string]interface{}{})

	assert.False(t, features.HasFriendsFamily())
	assert.False(t, features.HasPremiumAnalytics())
}

func TestTenantFeatures_FromMap_InvalidTypes_IgnoresBadValues(t *testing.T) {
	features := value_object.NewTenantFeatures()
	data := map[string]interface{}{
		"friends_family":    "not_a_bool",
		"premium_analytics": 42,
	}

	features.FromMap(data)

	assert.False(t, features.HasFriendsFamily())
	assert.False(t, features.HasPremiumAnalytics())
}

func TestTenantFeatures_RoundTrip_ToMapFromMap_PreservesValues(t *testing.T) {
	original := value_object.NewTenantFeaturesWithValues(true, true)

	exported := original.ToMap()

	restored := value_object.NewTenantFeatures()
	restored.FromMap(exported)

	assert.Equal(t, original.FriendsFamily, restored.FriendsFamily)
	assert.Equal(t, original.PremiumAnalytics, restored.PremiumAnalytics)
}
