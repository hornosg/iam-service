package usecase_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"iam/src/auth/application/usecase"
	"iam/src/auth/domain/value_object"
	"iam/src/auth/infrastructure/adapter"
)

func generateTestToken(secret string, claims *value_object.TokenClaims) string {
	svc := adapter.NewJWTServiceAdapter(secret)
	tokenStr, _ := svc.Sign(claims)
	return tokenStr
}

func TestValidateTokenUseCase_Execute_ValidToken_ReturnsClaims(t *testing.T) {
	// Arrange
	secret := "test-secret-key-for-testing-purposes"
	validateUseCase := usecase.NewValidateTokenUseCase(adapter.NewJWTServiceAdapter(secret))

	userID := uuid.New()
	tenantID := uuid.New()
	roleID := uuid.New()

	claims := value_object.NewTokenClaims(
		userID,
		tenantID,
		roleID,
		"test@example.com",
		"mc",
		value_object.DefaultTenantFeatures(),
		time.Now().Add(15*time.Minute),
	)

	tokenStr := generateTestToken(secret, claims)

	// Act
	result, err := validateUseCase.Execute(tokenStr)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, userID, result.UserID)
	assert.Equal(t, tenantID, result.TenantID)
	assert.Equal(t, roleID, result.RoleID)
	assert.Equal(t, "test@example.com", result.Email)
}

func TestValidateTokenUseCase_Execute_ExpiredToken_ReturnsError(t *testing.T) {
	// Arrange
	secret := "test-secret-key-for-testing-purposes"
	validateUseCase := usecase.NewValidateTokenUseCase(adapter.NewJWTServiceAdapter(secret))

	claims := value_object.NewTokenClaims(
		uuid.New(),
		uuid.New(),
		uuid.New(),
		"test@example.com",
		"mc",
		value_object.DefaultTenantFeatures(),
		time.Now().Add(-1*time.Hour), // Ya expirado
	)

	tokenStr := generateTestToken(secret, claims)

	// Act
	result, err := validateUseCase.Execute(tokenStr)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "token")
}

func TestValidateTokenUseCase_Execute_InvalidFormat_ReturnsError(t *testing.T) {
	// Arrange
	validateUseCase := usecase.NewValidateTokenUseCase(adapter.NewJWTServiceAdapter("test-secret"))

	// Act
	result, err := validateUseCase.Execute("not-a-valid-jwt-token")

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestValidateTokenUseCase_Execute_WrongSecret_ReturnsError(t *testing.T) {
	// Arrange
	validateUseCase := usecase.NewValidateTokenUseCase(adapter.NewJWTServiceAdapter("correct-secret"))

	claims := value_object.NewTokenClaims(
		uuid.New(),
		uuid.New(),
		uuid.New(),
		"test@example.com",
		"mc",
		value_object.DefaultTenantFeatures(),
		time.Now().Add(15*time.Minute),
	)

	// Firmar con un secret diferente
	tokenStr := generateTestToken("wrong-secret", claims)

	// Act
	result, err := validateUseCase.Execute(tokenStr)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestValidateTokenUseCase_Execute_EmptyToken_ReturnsError(t *testing.T) {
	// Arrange
	validateUseCase := usecase.NewValidateTokenUseCase(adapter.NewJWTServiceAdapter("test-secret"))

	// Act
	result, err := validateUseCase.Execute("")

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
}
