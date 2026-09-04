package value_object_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"iam/src/plans/domain/value_object"
)

func TestPlanType_AllTypes_AreValid(t *testing.T) {
	types := []value_object.PlanType{
		value_object.PlanTypeFree,
		value_object.PlanTypeBasic,
		value_object.PlanTypePremium,
		value_object.PlanTypeEnterprise,
	}

	for _, pt := range types {
		t.Run(string(pt), func(t *testing.T) {
			assert.True(t, pt.IsValid())
		})
	}
}

func TestPlanType_InvalidType_IsNotValid(t *testing.T) {
	invalidTypes := []value_object.PlanType{
		"UNKNOWN",
		"free",
		"",
		"STARTER",
	}

	for _, pt := range invalidTypes {
		t.Run(string(pt), func(t *testing.T) {
			assert.False(t, pt.IsValid())
		})
	}
}

func TestNewPlanTypeFromString_ValidString_ReturnsType(t *testing.T) {
	tests := []struct {
		input    string
		expected value_object.PlanType
	}{
		{"FREE", value_object.PlanTypeFree},
		{"BASIC", value_object.PlanTypeBasic},
		{"PREMIUM", value_object.PlanTypePremium},
		{"ENTERPRISE", value_object.PlanTypeEnterprise},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := value_object.NewPlanTypeFromString(tt.input)

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewPlanTypeFromString_InvalidString_ReturnsError(t *testing.T) {
	result, err := value_object.NewPlanTypeFromString("INVALID")

	assert.Error(t, err)
	assert.Equal(t, value_object.PlanType(""), result)
}

func TestPlanType_IsFree_OnlyFreeReturnsTrue(t *testing.T) {
	assert.True(t, value_object.PlanTypeFree.IsFree())
	assert.False(t, value_object.PlanTypeBasic.IsFree())
	assert.False(t, value_object.PlanTypePremium.IsFree())
	assert.False(t, value_object.PlanTypeEnterprise.IsFree())
}

func TestPlanType_GetMaxUsers_ReturnsCorrectLimits(t *testing.T) {
	tests := []struct {
		planType value_object.PlanType
		expected int
	}{
		{value_object.PlanTypeFree, 1},
		{value_object.PlanTypeBasic, 10},
		{value_object.PlanTypePremium, 100},
		{value_object.PlanTypeEnterprise, -1}, // Unlimited
	}

	for _, tt := range tests {
		t.Run(string(tt.planType), func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.planType.GetMaxUsers())
		})
	}
}

func TestPlanType_AllowsMultipleUsers_AllExceptFree(t *testing.T) {
	assert.False(t, value_object.PlanTypeFree.AllowsMultipleUsers())
	assert.True(t, value_object.PlanTypeBasic.AllowsMultipleUsers())
	assert.True(t, value_object.PlanTypePremium.AllowsMultipleUsers())
	assert.True(t, value_object.PlanTypeEnterprise.AllowsMultipleUsers())
}

func TestPlanType_String_ReturnsStringRepresentation(t *testing.T) {
	assert.Equal(t, "FREE", value_object.PlanTypeFree.String())
	assert.Equal(t, "ENTERPRISE", value_object.PlanTypeEnterprise.String())
}
