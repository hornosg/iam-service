package value_object_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hornosg/iam-service/src/tenancy/domain/value_object"
)

func TestTenantStatus_AllStatuses_AreValid(t *testing.T) {
	statuses := []value_object.TenantStatus{
		value_object.TenantStatusActive,
		value_object.TenantStatusInactive,
		value_object.TenantStatusSuspended,
		value_object.TenantStatusDeleted,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			assert.True(t, status.IsValid())
		})
	}
}

func TestTenantStatus_InvalidStatus_IsNotValid(t *testing.T) {
	invalidStatuses := []value_object.TenantStatus{
		"UNKNOWN",
		"active",
		"",
		"BANNED",
	}

	for _, status := range invalidStatuses {
		t.Run(string(status), func(t *testing.T) {
			assert.False(t, status.IsValid())
		})
	}
}

func TestNewTenantStatusFromString_ValidString_ReturnsStatus(t *testing.T) {
	tests := []struct {
		input    string
		expected value_object.TenantStatus
	}{
		{"ACTIVE", value_object.TenantStatusActive},
		{"INACTIVE", value_object.TenantStatusInactive},
		{"SUSPENDED", value_object.TenantStatusSuspended},
		{"DELETED", value_object.TenantStatusDeleted},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			status, err := value_object.NewTenantStatusFromString(tt.input)

			require.NoError(t, err)
			assert.Equal(t, tt.expected, status)
		})
	}
}

func TestNewTenantStatusFromString_InvalidString_ReturnsError(t *testing.T) {
	status, err := value_object.NewTenantStatusFromString("INVALID")

	assert.Error(t, err)
	assert.Equal(t, value_object.TenantStatus(""), status)
}

func TestTenantStatus_IsActive_OnlyActiveReturnsTrue(t *testing.T) {
	assert.True(t, value_object.TenantStatusActive.IsActive())
	assert.False(t, value_object.TenantStatusInactive.IsActive())
	assert.False(t, value_object.TenantStatusSuspended.IsActive())
	assert.False(t, value_object.TenantStatusDeleted.IsActive())
}

func TestTenantStatus_CanAccess_OnlyActiveReturnsTrue(t *testing.T) {
	assert.True(t, value_object.TenantStatusActive.CanAccess())
	assert.False(t, value_object.TenantStatusInactive.CanAccess())
	assert.False(t, value_object.TenantStatusSuspended.CanAccess())
	assert.False(t, value_object.TenantStatusDeleted.CanAccess())
}

func TestTenantStatus_CanBeModified_AllExceptDeleted(t *testing.T) {
	assert.True(t, value_object.TenantStatusActive.CanBeModified())
	assert.True(t, value_object.TenantStatusInactive.CanBeModified())
	assert.True(t, value_object.TenantStatusSuspended.CanBeModified())
	assert.False(t, value_object.TenantStatusDeleted.CanBeModified())
}

func TestTenantStatus_String_ReturnsStringRepresentation(t *testing.T) {
	assert.Equal(t, "ACTIVE", value_object.TenantStatusActive.String())
	assert.Equal(t, "DELETED", value_object.TenantStatusDeleted.String())
}
