package value_object_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"iam/src/identity/domain/value_object"
)

func TestNewEmail_ValidEmails_ReturnsEmail(t *testing.T) {
	validEmails := []string{
		"user@example.com",
		"test.user@domain.co",
		"name+tag@company.org",
		"user123@sub.domain.com",
		"first.last@example.com",
	}

	for _, email := range validEmails {
		t.Run(email, func(t *testing.T) {
			result, err := value_object.NewEmail(email)

			require.NoError(t, err)
			assert.Equal(t, email, result.Value())
			assert.Equal(t, email, result.String())
		})
	}
}

func TestNewEmail_InvalidEmails_ReturnsError(t *testing.T) {
	invalidEmails := []string{
		"",
		"invalid",
		"@domain.com",
		"user@",
		"user@.com",
		"user@domain",
		"user domain@example.com",
	}

	for _, email := range invalidEmails {
		t.Run(email, func(t *testing.T) {
			result, err := value_object.NewEmail(email)

			assert.Error(t, err)
			assert.Nil(t, result)
		})
	}
}

func TestNewEmail_WithSpaces_TrimsAndValidates(t *testing.T) {
	result, err := value_object.NewEmail("  user@example.com  ")

	require.NoError(t, err)
	assert.Equal(t, "user@example.com", result.Value())
}

func TestEmail_Domain_ReturnsCorrectDomain(t *testing.T) {
	tests := []struct {
		email          string
		expectedDomain string
	}{
		{"user@example.com", "example.com"},
		{"admin@sub.domain.org", "sub.domain.org"},
		{"test@company.co", "company.co"},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			email, err := value_object.NewEmail(tt.email)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedDomain, email.Domain())
		})
	}
}

func TestEmail_MarshalJSON_ReturnsQuotedString(t *testing.T) {
	email, err := value_object.NewEmail("user@example.com")
	require.NoError(t, err)

	data, err := email.MarshalJSON()

	require.NoError(t, err)
	assert.Equal(t, `"user@example.com"`, string(data))
}

func TestEmail_UnmarshalJSON_ValidEmail_Succeeds(t *testing.T) {
	var email value_object.Email

	err := email.UnmarshalJSON([]byte(`"user@example.com"`))

	require.NoError(t, err)
	assert.Equal(t, "user@example.com", email.Value())
}

func TestEmail_UnmarshalJSON_InvalidEmail_ReturnsError(t *testing.T) {
	var email value_object.Email

	err := email.UnmarshalJSON([]byte(`"invalid"`))

	assert.Error(t, err)
}

func TestEmail_UnmarshalJSON_InvalidJSON_ReturnsError(t *testing.T) {
	var email value_object.Email

	err := email.UnmarshalJSON([]byte(`not-json`))

	assert.Error(t, err)
}
