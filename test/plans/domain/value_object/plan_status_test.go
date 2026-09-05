package value_object_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hornosg/iam-service/src/plans/domain/value_object"
)

func TestPlanStatus_AllStatuses_AreValid(t *testing.T) {
	statuses := []value_object.PlanStatus{
		value_object.PlanStatusActive,
		value_object.PlanStatusInactive,
		value_object.PlanStatusDeprecated,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			assert.True(t, status.IsValid())
		})
	}
}

func TestPlanStatus_InvalidStatus_IsNotValid(t *testing.T) {
	invalidStatuses := []value_object.PlanStatus{
		"UNKNOWN",
		"active",
		"",
		"DELETED",
	}

	for _, status := range invalidStatuses {
		t.Run(string(status), func(t *testing.T) {
			assert.False(t, status.IsValid())
		})
	}
}

func TestNewPlanStatusFromString_ValidString_ReturnsStatus(t *testing.T) {
	tests := []struct {
		input    string
		expected value_object.PlanStatus
	}{
		{"ACTIVE", value_object.PlanStatusActive},
		{"INACTIVE", value_object.PlanStatusInactive},
		{"DEPRECATED", value_object.PlanStatusDeprecated},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			status, err := value_object.NewPlanStatusFromString(tt.input)

			require.NoError(t, err)
			assert.Equal(t, tt.expected, status)
		})
	}
}

func TestNewPlanStatusFromString_InvalidString_ReturnsError(t *testing.T) {
	status, err := value_object.NewPlanStatusFromString("INVALID")

	assert.Error(t, err)
	assert.Equal(t, value_object.PlanStatus(""), status)
}

func TestPlanStatus_IsActive_OnlyActiveReturnsTrue(t *testing.T) {
	assert.True(t, value_object.PlanStatusActive.IsActive())
	assert.False(t, value_object.PlanStatusInactive.IsActive())
	assert.False(t, value_object.PlanStatusDeprecated.IsActive())
}

func TestPlanStatus_CanBeAssigned_OnlyActiveReturnsTrue(t *testing.T) {
	assert.True(t, value_object.PlanStatusActive.CanBeAssigned())
	assert.False(t, value_object.PlanStatusInactive.CanBeAssigned())
	assert.False(t, value_object.PlanStatusDeprecated.CanBeAssigned())
}

func TestPlanStatus_String_ReturnsStringRepresentation(t *testing.T) {
	assert.Equal(t, "ACTIVE", value_object.PlanStatusActive.String())
	assert.Equal(t, "DEPRECATED", value_object.PlanStatusDeprecated.String())
}
