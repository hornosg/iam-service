package value_object_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"iam/src/identity/domain/value_object"
)

func TestAuthProvider_Local_IsValid(t *testing.T) {
	assert.True(t, value_object.LocalAuth.IsValid())
}

func TestAuthProvider_Google_IsValid(t *testing.T) {
	assert.True(t, value_object.GoogleAuth.IsValid())
}

func TestAuthProvider_Invalid_IsNotValid(t *testing.T) {
	invalidProviders := []value_object.AuthProvider{
		"FACEBOOK",
		"GITHUB",
		"",
		"local",
	}

	for _, provider := range invalidProviders {
		t.Run(string(provider), func(t *testing.T) {
			assert.False(t, provider.IsValid())
		})
	}
}

func TestAuthProvider_String_ReturnsStringRepresentation(t *testing.T) {
	assert.Equal(t, "LOCAL", value_object.LocalAuth.String())
	assert.Equal(t, "GOOGLE", value_object.GoogleAuth.String())
}

func TestAuthProvider_Constants_HaveExpectedValues(t *testing.T) {
	assert.Equal(t, value_object.AuthProvider("LOCAL"), value_object.LocalAuth)
	assert.Equal(t, value_object.AuthProvider("GOOGLE"), value_object.GoogleAuth)
}
