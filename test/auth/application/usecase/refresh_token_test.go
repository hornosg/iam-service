package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"iam/src/auth/application/usecase"
	"iam/src/auth/domain/entity"
	"iam/src/auth/domain/port"
	tenant_vo "iam/src/tenant/domain/value_object"
	authEntity "iam/test/auth/domain/entity"
	"iam/test/auth/infrastructure/persistence/repository"
)

// MockUserService implementa port.UserService para pruebas
type MockUserService struct {
	users       map[uuid.UUID]*port.UserData
	shouldFail  bool
	callHistory map[string]int
}

func NewMockUserService() *MockUserService {
	return &MockUserService{
		users:       make(map[uuid.UUID]*port.UserData),
		callHistory: make(map[string]int),
	}
}

func (m *MockUserService) SetupUser(user *port.UserData) {
	m.users[user.ID] = user
}

func (m *MockUserService) SetShouldFail(shouldFail bool) {
	m.shouldFail = shouldFail
}

func (m *MockUserService) GetCallCount(method string) int {
	return m.callHistory[method]
}

func (m *MockUserService) FindUserByID(ctx context.Context, id uuid.UUID) (*port.UserData, error) {
	m.callHistory["FindUserByID"]++
	if m.shouldFail {
		return nil, usecase.ErrUserNotFound
	}
	user, exists := m.users[id]
	if !exists {
		return nil, usecase.ErrUserNotFound
	}
	return user, nil
}

func (m *MockUserService) FindUserByEmail(ctx context.Context, email string, tenantID *uuid.UUID) (*port.UserData, error) {
	m.callHistory["FindUserByEmail"]++
	if m.shouldFail {
		return nil, usecase.ErrUserNotFound
	}
	for _, user := range m.users {
		if user.Email == email {
			if tenantID == nil || user.TenantID == *tenantID {
				return user, nil
			}
		}
	}
	return nil, usecase.ErrUserNotFound
}

// MockTenantService implementa port.TenantService para pruebas
type MockTenantService struct {
	features    *tenant_vo.TenantFeatures
	shouldFail  bool
	callHistory map[string]int
}

func NewMockTenantService() *MockTenantService {
	return &MockTenantService{
		features: &tenant_vo.TenantFeatures{
			FriendsFamily:    true,
			PremiumAnalytics: false,
		},
		callHistory: make(map[string]int),
	}
}

func (m *MockTenantService) SetFeatures(features *tenant_vo.TenantFeatures) {
	m.features = features
}

func (m *MockTenantService) SetShouldFail(shouldFail bool) {
	m.shouldFail = shouldFail
}

func (m *MockTenantService) GetCallCount(method string) int {
	return m.callHistory[method]
}

func (m *MockTenantService) Execute(ctx context.Context, tenantID uuid.UUID) (*tenant_vo.TenantFeatures, error) {
	m.callHistory["Execute"]++
	if m.shouldFail {
		return nil, assert.AnError
	}
	return m.features, nil
}

func TestRefreshTokenUseCase_Execute(t *testing.T) {
	ctx := context.Background()
	tokenMother := authEntity.Create()

	t.Run("debería renovar token con éxito", func(t *testing.T) {
		// Arrange
		mockAuthRepo := repository.NewMockAuthRepository()
		mockUserService := NewMockUserService()
		mockTenantService := NewMockTenantService()

		config := usecase.AuthConfig{
			JWTSecret:          "test-secret",
			AccessTokenExpiry:  15 * time.Minute,
			RefreshTokenExpiry: 7 * 24 * time.Hour,
		}

		refreshTokenUseCase := usecase.NewRefreshTokenUseCase(
			config,
			mockAuthRepo,
			mockUserService,
			mockTenantService,
		)

		userID := uuid.New()
		tenantID := uuid.New()
		roleID := uuid.New()

		// Configurar usuario mock
		user := &port.UserData{
			ID:       userID,
			Email:    "test@example.com",
			TenantID: tenantID,
			RoleID:   roleID,
			Status:   "ACTIVE",
		}
		mockUserService.SetupUser(user)

		// Crear refresh token válido
		refreshToken := tokenMother.WithUser(userID)
		refreshToken.Token = "valid_refresh_token"
		mockAuthRepo.SetupRefreshTokens([]*entity.RefreshToken{refreshToken})

		// Act
		response, err := refreshTokenUseCase.Execute(ctx, refreshToken.Token)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.NotEmpty(t, response.AccessToken)
		assert.NotEmpty(t, response.RefreshToken)
		assert.Equal(t, "Bearer", response.TokenType)
		assert.Equal(t, user.ID, response.User.ID)
		assert.Equal(t, user.Email, response.User.Email)

		// Verificar llamadas
		assert.Equal(t, 1, mockAuthRepo.GetCallCount("GetRefreshToken"))
		assert.Equal(t, 1, mockUserService.GetCallCount("FindUserByID"))
		assert.Equal(t, 1, mockAuthRepo.GetCallCount("CreateRefreshToken"))
		assert.Equal(t, 1, mockAuthRepo.GetCallCount("DeleteRefreshToken"))
	})

	t.Run("debería fallar con token inexistente", func(t *testing.T) {
		// Arrange
		mockAuthRepo := repository.NewMockAuthRepository()
		mockUserService := NewMockUserService()
		mockTenantService := NewMockTenantService()

		config := usecase.AuthConfig{
			JWTSecret:          "test-secret",
			AccessTokenExpiry:  15 * time.Minute,
			RefreshTokenExpiry: 7 * 24 * time.Hour,
		}

		refreshTokenUseCase := usecase.NewRefreshTokenUseCase(
			config,
			mockAuthRepo,
			mockUserService,
			mockTenantService,
		)

		// Act
		response, err := refreshTokenUseCase.Execute(ctx, "token_inexistente")

		// Assert
		assert.Error(t, err)
		assert.Nil(t, response)
		assert.Equal(t, usecase.ErrInvalidToken, err)
		assert.Equal(t, 1, mockAuthRepo.GetCallCount("GetRefreshToken"))
		assert.Equal(t, 0, mockUserService.GetCallCount("FindUserByID"))
	})

	t.Run("debería fallar con token expirado", func(t *testing.T) {
		// Arrange
		mockAuthRepo := repository.NewMockAuthRepository()
		mockUserService := NewMockUserService()
		mockTenantService := NewMockTenantService()

		config := usecase.AuthConfig{
			JWTSecret:          "test-secret",
			AccessTokenExpiry:  15 * time.Minute,
			RefreshTokenExpiry: 7 * 24 * time.Hour,
		}

		refreshTokenUseCase := usecase.NewRefreshTokenUseCase(
			config,
			mockAuthRepo,
			mockUserService,
			mockTenantService,
		)

		userID := uuid.New()

		// Crear refresh token expirado
		expiredToken := tokenMother.Expired()
		expiredToken.UserID = userID
		mockAuthRepo.SetupRefreshTokens([]*entity.RefreshToken{expiredToken})

		// Act
		response, err := refreshTokenUseCase.Execute(ctx, expiredToken.Token)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, response)
		assert.Equal(t, usecase.ErrExpiredToken, err)
		assert.Equal(t, 1, mockAuthRepo.GetCallCount("GetRefreshToken"))
		assert.Equal(t, 1, mockAuthRepo.GetCallCount("DeleteRefreshToken")) // Token expirado se elimina
		assert.Equal(t, 0, mockUserService.GetCallCount("FindUserByID"))
	})

	t.Run("debería fallar si el usuario no existe", func(t *testing.T) {
		// Arrange
		mockAuthRepo := repository.NewMockAuthRepository()
		mockUserService := NewMockUserService()
		mockTenantService := NewMockTenantService()

		config := usecase.AuthConfig{
			JWTSecret:          "test-secret",
			AccessTokenExpiry:  15 * time.Minute,
			RefreshTokenExpiry: 7 * 24 * time.Hour,
		}

		refreshTokenUseCase := usecase.NewRefreshTokenUseCase(
			config,
			mockAuthRepo,
			mockUserService,
			mockTenantService,
		)

		userID := uuid.New()

		// Crear refresh token válido pero usuario inexistente
		refreshToken := tokenMother.WithUser(userID)
		mockAuthRepo.SetupRefreshTokens([]*entity.RefreshToken{refreshToken})

		// Act
		response, err := refreshTokenUseCase.Execute(ctx, refreshToken.Token)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, response)
		assert.Equal(t, usecase.ErrUserNotFound, err)
		assert.Equal(t, 1, mockAuthRepo.GetCallCount("GetRefreshToken"))
		assert.Equal(t, 1, mockUserService.GetCallCount("FindUserByID"))
		assert.Equal(t, 0, mockAuthRepo.GetCallCount("CreateRefreshToken"))
	})

	t.Run("debería fallar si GetRefreshToken falla", func(t *testing.T) {
		// Arrange
		mockAuthRepo := repository.NewMockAuthRepository()
		mockUserService := NewMockUserService()
		mockTenantService := NewMockTenantService()

		config := usecase.AuthConfig{
			JWTSecret:          "test-secret",
			AccessTokenExpiry:  15 * time.Minute,
			RefreshTokenExpiry: 7 * 24 * time.Hour,
		}

		refreshTokenUseCase := usecase.NewRefreshTokenUseCase(
			config,
			mockAuthRepo,
			mockUserService,
			mockTenantService,
		)

		mockAuthRepo.ShouldFailOn("GetRefreshToken")

		// Act
		response, err := refreshTokenUseCase.Execute(ctx, "any_token")

		// Assert
		assert.Error(t, err)
		assert.Nil(t, response)
		assert.Equal(t, usecase.ErrInvalidToken, err)
		assert.Equal(t, 1, mockAuthRepo.GetCallCount("GetRefreshToken"))
		assert.Equal(t, 0, mockUserService.GetCallCount("FindUserByID"))
	})

	t.Run("debería manejar fallo en creación de nuevo refresh token", func(t *testing.T) {
		// Arrange
		mockAuthRepo := repository.NewMockAuthRepository()
		mockUserService := NewMockUserService()
		mockTenantService := NewMockTenantService()

		config := usecase.AuthConfig{
			JWTSecret:          "test-secret",
			AccessTokenExpiry:  15 * time.Minute,
			RefreshTokenExpiry: 7 * 24 * time.Hour,
		}

		refreshTokenUseCase := usecase.NewRefreshTokenUseCase(
			config,
			mockAuthRepo,
			mockUserService,
			mockTenantService,
		)

		mockAuthRepo.ShouldFailOn("CreateRefreshToken")

		userID := uuid.New()
		tenantID := uuid.New()
		roleID := uuid.New()

		// Configurar usuario mock
		user := &port.UserData{
			ID:       userID,
			Email:    "test@example.com",
			TenantID: tenantID,
			RoleID:   roleID,
			Status:   "ACTIVE",
		}
		mockUserService.SetupUser(user)

		// Crear refresh token válido
		refreshToken := tokenMother.WithUser(userID)
		mockAuthRepo.SetupRefreshTokens([]*entity.RefreshToken{refreshToken})

		// Act
		response, err := refreshTokenUseCase.Execute(ctx, refreshToken.Token)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, response)
		assert.Equal(t, 1, mockAuthRepo.GetCallCount("GetRefreshToken"))
		assert.Equal(t, 1, mockUserService.GetCallCount("FindUserByID"))
		assert.Equal(t, 1, mockAuthRepo.GetCallCount("CreateRefreshToken"))
	})

	t.Run("debería funcionar aunque falle obtener features del tenant", func(t *testing.T) {
		// Arrange
		mockAuthRepo := repository.NewMockAuthRepository()
		mockUserService := NewMockUserService()
		mockTenantService := NewMockTenantService()

		config := usecase.AuthConfig{
			JWTSecret:          "test-secret",
			AccessTokenExpiry:  15 * time.Minute,
			RefreshTokenExpiry: 7 * 24 * time.Hour,
		}

		refreshTokenUseCase := usecase.NewRefreshTokenUseCase(
			config,
			mockAuthRepo,
			mockUserService,
			mockTenantService,
		)

		mockTenantService.SetShouldFail(true)

		userID := uuid.New()
		tenantID := uuid.New()
		roleID := uuid.New()

		// Configurar usuario mock
		user := &port.UserData{
			ID:       userID,
			Email:    "test@example.com",
			TenantID: tenantID,
			RoleID:   roleID,
			Status:   "ACTIVE",
		}
		mockUserService.SetupUser(user)

		// Crear refresh token válido
		refreshToken := tokenMother.WithUser(userID)
		mockAuthRepo.SetupRefreshTokens([]*entity.RefreshToken{refreshToken})

		// Act
		response, err := refreshTokenUseCase.Execute(ctx, refreshToken.Token)

		// Assert - Debería funcionar con features por defecto
		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.NotEmpty(t, response.AccessToken)
		assert.Equal(t, 1, mockTenantService.GetCallCount("Execute"))
	})
}
