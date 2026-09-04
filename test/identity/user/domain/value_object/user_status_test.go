package value_object_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"iam/src/identity/domain/value_object"
)

func TestUserStatus_AllStatuses_AreValid(t *testing.T) {
	statuses := []value_object.UserStatus{
		value_object.StatusActive,
		value_object.StatusInactive,
		value_object.StatusPending,
		value_object.StatusBlocked,
		value_object.StatusDeleted,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			assert.True(t, status.IsValid())
		})
	}
}

func TestUserStatus_InvalidStatus_IsNotValid(t *testing.T) {
	invalidStatuses := []value_object.UserStatus{
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

func TestNewUserStatusFromString_ValidString_ReturnsStatus(t *testing.T) {
	tests := []struct {
		input    string
		expected value_object.UserStatus
	}{
		{"ACTIVE", value_object.StatusActive},
		{"INACTIVE", value_object.StatusInactive},
		{"PENDING", value_object.StatusPending},
		{"BLOCKED", value_object.StatusBlocked},
		{"DELETED", value_object.StatusDeleted},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			status, err := value_object.NewUserStatusFromString(tt.input)

			require.NoError(t, err)
			assert.Equal(t, tt.expected, status)
		})
	}
}

func TestNewUserStatusFromString_InvalidString_ReturnsError(t *testing.T) {
	status, err := value_object.NewUserStatusFromString("INVALID")

	assert.Error(t, err)
	assert.Equal(t, value_object.UserStatus(""), status)
}

func TestUserStatus_String_ReturnsStringRepresentation(t *testing.T) {
	assert.Equal(t, "ACTIVE", value_object.StatusActive.String())
	assert.Equal(t, "BLOCKED", value_object.StatusBlocked.String())
}
