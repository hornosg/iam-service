package usecase_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"iam/src/auth/application/request"
	"iam/src/auth/application/usecase"
	"iam/src/auth/domain/port"
	"iam/src/auth/domain/value_object"
	"iam/src/auth/infrastructure/adapter"
	sharedlog "github.com/mercadocercano/go-shared/infrastructure/logging"
	"iam/test/auth/infrastructure/persistence/repository"
)

type MockGoogleTokenVerifier struct {
	claims port.GoogleClaims
	err    error
}

func NewMockGoogleTokenVerifier() *MockGoogleTokenVerifier {
	return &MockGoogleTokenVerifier{}
}

func (m *MockGoogleTokenVerifier) SetupClaims(claims port.GoogleClaims) {
	m.claims = claims
	m.err = nil
}

func (m *MockGoogleTokenVerifier) SetupError(err error) {
	m.err = err
}

func (m *MockGoogleTokenVerifier) Verify(_ context.Context, _ string) (port.GoogleClaims, error) {
	if m.err != nil {
		return port.GoogleClaims{}, m.err
	}
	return m.claims, nil
}

func TestLoginUseCase_Execute_ValidLocalLogin_ReturnsTokens(t *testing.T) {
	// Arrange
	mockAuthRepo := repository.NewMockAuthRepository()
	mockUserService := NewMockUserService()
	mockTenantService := NewMockTenantService()

	config := usecase.AuthConfig{
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
	}
	jwtSvc := adapter.NewJWTServiceAdapter("test-secret-key-for-testing-purposes")

	loginUseCase := usecase.NewLoginUseCase(config, mockAuthRepo, mockUserService, mockTenantService, jwtSvc, NewMockGoogleTokenVerifier(), sharedlog.NewSecurityLoggerWithWriter("iam-test", io.Discard))

	userID := uuid.New()
	tenantID := uuid.New()
	roleID := uuid.New()

	// Generar hash de password valido
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)

	user := &port.UserData{
		ID:           userID,
		Email:        "test@example.com",
		PasswordHash: string(passwordHash),
		TenantID:     tenantID,
		RoleID:       roleID,
		Status:       "ACTIVE",
		Provider:     "LOCAL",
	}
	mockUserService.SetupUser(user)

	loginReq := &request.LoginRequest{
		Email:    "test@example.com",
		Password: "password123",
		Provider: value_object.LocalAuth,
		TenantID: &tenantID,
	}

	// Act
	resp, err := loginUseCase.Execute(context.Background(), loginReq)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.NotEmpty(t, resp.AccessToken)
	assert.NotEmpty(t, resp.RefreshToken)
	assert.Equal(t, "Bearer", resp.TokenType)
	assert.Equal(t, userID, resp.User.ID)
	assert.Equal(t, "test@example.com", resp.User.Email)
	assert.Equal(t, tenantID, resp.User.TenantID)
	assert.Equal(t, roleID, resp.User.RoleID)
}

func TestLoginUseCase_Execute_InvalidPassword_ReturnsError(t *testing.T) {
	// Arrange
	mockAuthRepo := repository.NewMockAuthRepository()
	mockUserService := NewMockUserService()
	mockTenantService := NewMockTenantService()

	config := usecase.AuthConfig{
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
	}
	jwtSvc := adapter.NewJWTServiceAdapter("test-secret-key-for-testing-purposes")

	loginUseCase := usecase.NewLoginUseCase(config, mockAuthRepo, mockUserService, mockTenantService, jwtSvc, NewMockGoogleTokenVerifier(), sharedlog.NewSecurityLoggerWithWriter("iam-test", io.Discard))

	userID := uuid.New()
	tenantID := uuid.New()
	roleID := uuid.New()

	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("correctpassword"), bcrypt.DefaultCost)

	user := &port.UserData{
		ID:           userID,
		Email:        "test@example.com",
		PasswordHash: string(passwordHash),
		TenantID:     tenantID,
		RoleID:       roleID,
		Status:       "ACTIVE",
		Provider:     "LOCAL",
	}
	mockUserService.SetupUser(user)

	loginReq := &request.LoginRequest{
		Email:    "test@example.com",
		Password: "wrongpassword",
		Provider: value_object.LocalAuth,
		TenantID: &tenantID,
	}

	// Act
	resp, err := loginUseCase.Execute(context.Background(), loginReq)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, usecase.ErrInvalidCredentials, err)
}

func TestLoginUseCase_Execute_UserNotFound_ReturnsError(t *testing.T) {
	// Arrange
	mockAuthRepo := repository.NewMockAuthRepository()
	mockUserService := NewMockUserService()
	mockTenantService := NewMockTenantService()

	config := usecase.AuthConfig{
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
	}
	jwtSvc := adapter.NewJWTServiceAdapter("test-secret-key-for-testing-purposes")

	loginUseCase := usecase.NewLoginUseCase(config, mockAuthRepo, mockUserService, mockTenantService, jwtSvc, NewMockGoogleTokenVerifier(), sharedlog.NewSecurityLoggerWithWriter("iam-test", io.Discard))

	tenantID := uuid.New()

	loginReq := &request.LoginRequest{
		Email:    "nonexistent@example.com",
		Password: "password123",
		Provider: value_object.LocalAuth,
		TenantID: &tenantID,
	}

	// Act
	resp, err := loginUseCase.Execute(context.Background(), loginReq)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, usecase.ErrInvalidCredentials, err)
	assert.Equal(t, 1, mockUserService.GetCallCount("FindUserByEmail"))
}

func TestLoginUseCase_Execute_GoogleUser_WithLocalAuth_ReturnsError(t *testing.T) {
	// Arrange
	mockAuthRepo := repository.NewMockAuthRepository()
	mockUserService := NewMockUserService()
	mockTenantService := NewMockTenantService()

	config := usecase.AuthConfig{
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
	}
	jwtSvc := adapter.NewJWTServiceAdapter("test-secret-key-for-testing-purposes")

	loginUseCase := usecase.NewLoginUseCase(config, mockAuthRepo, mockUserService, mockTenantService, jwtSvc, NewMockGoogleTokenVerifier(), sharedlog.NewSecurityLoggerWithWriter("iam-test", io.Discard))

	userID := uuid.New()
	tenantID := uuid.New()
	roleID := uuid.New()

	user := &port.UserData{
		ID:           userID,
		Email:        "google@example.com",
		PasswordHash: "",
		TenantID:     tenantID,
		RoleID:       roleID,
		Status:       "ACTIVE",
		Provider:     "GOOGLE", // No es LOCAL
	}
	mockUserService.SetupUser(user)

	loginReq := &request.LoginRequest{
		Email:    "google@example.com",
		Password: "password123",
		Provider: value_object.LocalAuth,
		TenantID: &tenantID,
	}

	// Act
	resp, err := loginUseCase.Execute(context.Background(), loginReq)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "GOOGLE")
}

func TestLoginUseCase_Execute_InvalidProvider_ReturnsError(t *testing.T) {
	// Arrange
	mockAuthRepo := repository.NewMockAuthRepository()
	mockUserService := NewMockUserService()
	mockTenantService := NewMockTenantService()

	config := usecase.AuthConfig{
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
	}
	jwtSvc := adapter.NewJWTServiceAdapter("test-secret")

	loginUseCase := usecase.NewLoginUseCase(config, mockAuthRepo, mockUserService, mockTenantService, jwtSvc, NewMockGoogleTokenVerifier(), sharedlog.NewSecurityLoggerWithWriter("iam-test", io.Discard))

	loginReq := &request.LoginRequest{
		Email:    "test@example.com",
		Password: "password123",
		Provider: value_object.AuthProvider("INVALID"),
	}

	// Act
	resp, err := loginUseCase.Execute(context.Background(), loginReq)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestLoginUseCase_Execute_MissingPassword_ReturnsError(t *testing.T) {
	// Arrange
	mockAuthRepo := repository.NewMockAuthRepository()
	mockUserService := NewMockUserService()
	mockTenantService := NewMockTenantService()

	config := usecase.AuthConfig{
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
	}
	jwtSvc := adapter.NewJWTServiceAdapter("test-secret")

	loginUseCase := usecase.NewLoginUseCase(config, mockAuthRepo, mockUserService, mockTenantService, jwtSvc, NewMockGoogleTokenVerifier(), sharedlog.NewSecurityLoggerWithWriter("iam-test", io.Discard))

	loginReq := &request.LoginRequest{
		Email:    "test@example.com",
		Password: "",
		Provider: value_object.LocalAuth,
	}

	// Act
	resp, err := loginUseCase.Execute(context.Background(), loginReq)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, request.ErrPasswordRequired, err)
}

func TestLoginUseCase_Execute_TenantMismatch_ReturnsError(t *testing.T) {
	// Arrange
	mockAuthRepo := repository.NewMockAuthRepository()
	mockUserService := NewMockUserService()
	mockTenantService := NewMockTenantService()

	config := usecase.AuthConfig{
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
	}
	jwtSvc := adapter.NewJWTServiceAdapter("test-secret-key-for-testing-purposes")

	loginUseCase := usecase.NewLoginUseCase(config, mockAuthRepo, mockUserService, mockTenantService, jwtSvc, NewMockGoogleTokenVerifier(), sharedlog.NewSecurityLoggerWithWriter("iam-test", io.Discard))

	userID := uuid.New()
	userTenantID := uuid.New()
	requestTenantID := uuid.New()
	roleID := uuid.New()

	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)

	user := &port.UserData{
		ID:           userID,
		Email:        "test@example.com",
		PasswordHash: string(passwordHash),
		TenantID:     userTenantID,
		RoleID:       roleID,
		Status:       "ACTIVE",
		Provider:     "LOCAL",
	}
	mockUserService.SetupUser(user)

	loginReq := &request.LoginRequest{
		Email:    "test@example.com",
		Password: "password123",
		Provider: value_object.LocalAuth,
		TenantID: &requestTenantID, // Diferente al tenant del usuario
	}

	// Act
	resp, err := loginUseCase.Execute(context.Background(), loginReq)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, usecase.ErrInvalidCredentials, err)
}

func TestLoginUseCase_Execute_GoogleLogin_ValidToken_ReturnsTokens(t *testing.T) {
	// Arrange
	mockAuthRepo := repository.NewMockAuthRepository()
	mockUserService := NewMockUserService()
	mockTenantService := NewMockTenantService()
	mockGoogleVerifier := NewMockGoogleTokenVerifier()

	config := usecase.AuthConfig{
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
	}
	jwtSvc := adapter.NewJWTServiceAdapter("test-secret-key-for-testing-purposes")

	loginUseCase := usecase.NewLoginUseCase(config, mockAuthRepo, mockUserService, mockTenantService, jwtSvc, mockGoogleVerifier, sharedlog.NewSecurityLoggerWithWriter("iam-test", io.Discard))

	userID := uuid.New()
	tenantID := uuid.New()
	roleID := uuid.New()

	mockGoogleVerifier.SetupClaims(port.GoogleClaims{
		Sub:   "google-sub-123",
		Email: "google@example.com",
	})

	user := &port.UserData{
		ID:       userID,
		Email:    "google@example.com",
		TenantID: tenantID,
		RoleID:   roleID,
		Status:   "ACTIVE",
		Provider: "GOOGLE",
	}
	mockUserService.SetupUser(user)

	loginReq := &request.LoginRequest{
		GoogleToken: "fake-google-id-token",
		Provider:    value_object.GoogleAuth,
		TenantID:    &tenantID,
	}

	// Act
	resp, err := loginUseCase.Execute(context.Background(), loginReq)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.NotEmpty(t, resp.AccessToken)
	assert.NotEmpty(t, resp.RefreshToken)
	assert.Equal(t, userID, resp.User.ID)
}

func TestLoginUseCase_Execute_GoogleLogin_InvalidToken_ReturnsError(t *testing.T) {
	// Arrange
	mockAuthRepo := repository.NewMockAuthRepository()
	mockUserService := NewMockUserService()
	mockTenantService := NewMockTenantService()
	mockGoogleVerifier := NewMockGoogleTokenVerifier()

	config := usecase.AuthConfig{
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
	}
	jwtSvc := adapter.NewJWTServiceAdapter("test-secret-key-for-testing-purposes")

	loginUseCase := usecase.NewLoginUseCase(config, mockAuthRepo, mockUserService, mockTenantService, jwtSvc, mockGoogleVerifier, sharedlog.NewSecurityLoggerWithWriter("iam-test", io.Discard))

	mockGoogleVerifier.SetupError(errors.New("token de Google inválido"))

	loginReq := &request.LoginRequest{
		GoogleToken: "bad-token",
		Provider:    value_object.GoogleAuth,
	}

	// Act
	resp, err := loginUseCase.Execute(context.Background(), loginReq)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "inválido")
}
