package entity

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	srcEntity "github.com/hornosg/iam-service/src/plans/domain/entity"
	"github.com/hornosg/iam-service/src/plans/domain/value_object"
)

func TestNewPlan_WithValidParams_CreatesWithDefaults(t *testing.T) {
	plan := srcEntity.NewPlan("Plan Test", "Descripcion", value_object.PlanTypeBasic, 9.99, 99.99)

	assert.NotEmpty(t, plan.ID)
	assert.Equal(t, "Plan Test", plan.Name)
	assert.Equal(t, value_object.PlanTypeBasic, plan.Type)
	assert.Equal(t, value_object.PlanStatusActive, plan.Status)
	assert.Equal(t, 10, plan.MaxUsers)
	assert.Equal(t, 9.99, plan.PriceMonth)
	assert.Equal(t, 99.99, plan.PriceYear)
	assert.Empty(t, plan.Features)
}

func TestNewPlan_FreeType_MaxUsersIsOne(t *testing.T) {
	plan := srcEntity.NewPlan("Free", "Desc", value_object.PlanTypeFree, 0, 0)

	assert.Equal(t, 1, plan.MaxUsers)
}

func TestNewPlan_EnterpriseType_MaxUsersIsUnlimited(t *testing.T) {
	plan := srcEntity.NewPlan("Enterprise", "Desc", value_object.PlanTypeEnterprise, 99.99, 999.99)

	assert.Equal(t, -1, plan.MaxUsers)
}

func TestPlan_UpdatePricing_ChangesPrices(t *testing.T) {
	mother := Create()
	plan := mother.WithDefaults()

	plan.UpdatePricing(19.99, 199.99)

	assert.Equal(t, 19.99, plan.PriceMonth)
	assert.Equal(t, 199.99, plan.PriceYear)
}

func TestPlan_ChangeStatus_ChangesStatus(t *testing.T) {
	mother := Create()
	plan := mother.WithDefaults()

	plan.ChangeStatus(value_object.PlanStatusInactive)

	assert.Equal(t, value_object.PlanStatusInactive, plan.Status)
}

func TestPlan_AddFeature_AddsToList(t *testing.T) {
	mother := Create()
	plan := mother.WithDefaults()

	plan.AddFeature("new_feature")

	assert.Contains(t, plan.Features, "new_feature")
}

func TestPlan_RemoveFeature_RemovesFromList(t *testing.T) {
	mother := Create()
	plan := mother.WithFeatures([]string{"feat1", "feat2", "feat3"})

	plan.RemoveFeature("feat2")

	assert.NotContains(t, plan.Features, "feat2")
	assert.Len(t, plan.Features, 2)
}

func TestPlan_RemoveFeature_NonExistent_DoesNothing(t *testing.T) {
	mother := Create()
	plan := mother.WithFeatures([]string{"feat1"})

	plan.RemoveFeature("nonexistent")

	assert.Len(t, plan.Features, 1)
}

func TestPlan_HasFeature_ExistingFeature_ReturnsTrue(t *testing.T) {
	mother := Create()
	plan := mother.WithFeatures([]string{"feat1", "feat2"})

	assert.True(t, plan.HasFeature("feat1"))
}

func TestPlan_HasFeature_NonExistentFeature_ReturnsFalse(t *testing.T) {
	mother := Create()
	plan := mother.WithDefaults()

	assert.False(t, plan.HasFeature("nonexistent"))
}

func TestPlan_IsActive_ActivePlan_ReturnsTrue(t *testing.T) {
	mother := Create()
	plan := mother.WithDefaults()

	assert.True(t, plan.IsActive())
}

func TestPlan_IsActive_InactivePlan_ReturnsFalse(t *testing.T) {
	mother := Create()
	plan := mother.Inactive()

	assert.False(t, plan.IsActive())
}

func TestPlan_CanBeAssigned_ActivePlan_ReturnsTrue(t *testing.T) {
	mother := Create()
	plan := mother.WithDefaults()

	assert.True(t, plan.CanBeAssigned())
}

func TestPlan_CanBeAssigned_DeprecatedPlan_ReturnsFalse(t *testing.T) {
	mother := Create()
	plan := mother.Deprecated()

	assert.False(t, plan.CanBeAssigned())
}

func TestPlan_IsFree_FreePlan_ReturnsTrue(t *testing.T) {
	mother := Create()
	plan := mother.Free()

	assert.True(t, plan.IsFree())
}

func TestPlan_IsFree_BasicPlan_ReturnsFalse(t *testing.T) {
	mother := Create()
	plan := mother.Basic()

	assert.False(t, plan.IsFree())
}

func TestPlan_AllowsUsers_WithinLimit_ReturnsTrue(t *testing.T) {
	mother := Create()
	plan := mother.WithDefaults() // Basic = 10

	assert.True(t, plan.AllowsUsers(5))
	assert.True(t, plan.AllowsUsers(10))
}

func TestPlan_AllowsUsers_ExceedsLimit_ReturnsFalse(t *testing.T) {
	mother := Create()
	plan := mother.WithDefaults()

	assert.False(t, plan.AllowsUsers(11))
}

func TestPlan_AllowsUsers_UnlimitedPlan_AlwaysReturnsTrue(t *testing.T) {
	mother := Create()
	plan := mother.Enterprise()

	assert.True(t, plan.AllowsUsers(1000))
}

func TestPlan_GetYearlyDiscount_WithDiscount_ReturnsPercentage(t *testing.T) {
	mother := Create()
	plan := mother.WithPricing(10.0, 100.0)

	discount := plan.GetYearlyDiscount()

	require.Greater(t, discount, 16.0)
	assert.Less(t, discount, 17.0)
}

func TestPlan_GetYearlyDiscount_NoDiscount_ReturnsZero(t *testing.T) {
	mother := Create()
	plan := mother.WithPricing(10.0, 120.0)

	assert.Equal(t, 0.0, plan.GetYearlyDiscount())
}

func TestPlan_GetYearlyDiscount_FreePlan_ReturnsZero(t *testing.T) {
	mother := Create()
	plan := mother.WithPricing(0.0, 0.0)

	assert.Equal(t, 0.0, plan.GetYearlyDiscount())
}

func TestPlan_GetYearlyDiscount_YearlyMoreExpensive_ReturnsZero(t *testing.T) {
	mother := Create()
	plan := mother.WithPricing(10.0, 130.0)

	assert.Equal(t, 0.0, plan.GetYearlyDiscount())
}
