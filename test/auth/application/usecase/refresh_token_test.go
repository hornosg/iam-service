package usecase_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"iam/src/auth/application/usecase"
	"iam/src/auth/domain/entity"
	"iam/src/auth/domain/port"
	auth_vo "iam/src/auth/domain/value_object"
	"iam/src/auth/infrastructure/adapter"
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
	features    *auth_vo.TenantFeatures
	shouldFail  bool
	callHistory map[string]int
}

func NewMockTenantService() *MockTenantService {
	return &MockTenantService{
		features: &auth_vo.TenantFeatures{
			FriendsFamily:    true,
			PremiumAnalytics: false,
		},
		callHistory: make(map[string]int),
	}
}

func (m *MockTenantService) SetFeatures(features *auth_vo.TenantFeatures) {
	m.features = features
}

func (m *MockTenantService) SetShouldFail(shouldFail bool) {
	m.shouldFail = shouldFail
}

func (m *MockTenantService) GetCallCount(method string) int {
	return m.callHistory[method]
}

func (m *MockTenantService) Execute(ctx context.Context, tenantID uuid.UUID) (*auth_vo.TenantFeatures, error) {
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
			AccessTokenExpiry:  15 * time.Minute,
			RefreshTokenExpiry: 7 * 24 * time.Hour,
		}
		jwtSvc := adapter.NewJWTServiceAdapter("test-secret")

		refreshTokenUseCase := usecase.NewRefreshTokenUseCase(
			config,
			mockAuthRepo,
			mockUserService,
			mockTenantService,
			jwtSvc,
			NewMockRoleResolver(),
			NewMockPlanResolver(),
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

		// Crear refresh token válido. T8e: la fila trae tenant denormalizado
		// (NOT NULL desde la migración 021) — el mock lo refleja.
		refreshToken := tokenMother.WithUser(userID)
		refreshToken.Token = "valid_refresh_token"
		refreshToken.TenantID = tenantID
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
			AccessTokenExpiry:  15 * time.Minute,
			RefreshTokenExpiry: 7 * 24 * time.Hour,
		}
		jwtSvc := adapter.NewJWTServiceAdapter("test-secret")

		refreshTokenUseCase := usecase.NewRefreshTokenUseCase(
			config,
			mockAuthRepo,
			mockUserService,
			mockTenantService,
			jwtSvc,
			NewMockRoleResolver(),
			NewMockPlanResolver(),
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
			AccessTokenExpiry:  15 * time.Minute,
			RefreshTokenExpiry: 7 * 24 * time.Hour,
		}
		jwtSvc := adapter.NewJWTServiceAdapter("test-secret")

		refreshTokenUseCase := usecase.NewRefreshTokenUseCase(
			config,
			mockAuthRepo,
			mockUserService,
			mockTenantService,
			jwtSvc,
			NewMockRoleResolver(),
			NewMockPlanResolver(),
		)

		userID := uuid.New()

		// Crear refresh token expirado (con tenant: es lo que el repo devuelve
		// desde la denormalización de T8e)
		expiredToken := tokenMother.Expired()
		expiredToken.UserID = userID
		expiredToken.TenantID = uuid.New()
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
			AccessTokenExpiry:  15 * time.Minute,
			RefreshTokenExpiry: 7 * 24 * time.Hour,
		}
		jwtSvc := adapter.NewJWTServiceAdapter("test-secret")

		refreshTokenUseCase := usecase.NewRefreshTokenUseCase(
			config,
			mockAuthRepo,
			mockUserService,
			mockTenantService,
			jwtSvc,
			NewMockRoleResolver(),
			NewMockPlanResolver(),
		)

		userID := uuid.New()

		// Crear refresh token válido pero usuario inexistente
		refreshToken := tokenMother.WithUser(userID)
		refreshToken.TenantID = uuid.New()
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
			AccessTokenExpiry:  15 * time.Minute,
			RefreshTokenExpiry: 7 * 24 * time.Hour,
		}
		jwtSvc := adapter.NewJWTServiceAdapter("test-secret")

		refreshTokenUseCase := usecase.NewRefreshTokenUseCase(
			config,
			mockAuthRepo,
			mockUserService,
			mockTenantService,
			jwtSvc,
			NewMockRoleResolver(),
			NewMockPlanResolver(),
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
			AccessTokenExpiry:  15 * time.Minute,
			RefreshTokenExpiry: 7 * 24 * time.Hour,
		}
		jwtSvc := adapter.NewJWTServiceAdapter("test-secret")

		refreshTokenUseCase := usecase.NewRefreshTokenUseCase(
			config,
			mockAuthRepo,
			mockUserService,
			mockTenantService,
			jwtSvc,
			NewMockRoleResolver(),
			NewMockPlanResolver(),
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

		// Crear refresh token válido (con tenant denormalizado, T8e)
		refreshToken := tokenMother.WithUser(userID)
		refreshToken.TenantID = tenantID
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
			AccessTokenExpiry:  15 * time.Minute,
			RefreshTokenExpiry: 7 * 24 * time.Hour,
		}
		jwtSvc := adapter.NewJWTServiceAdapter("test-secret")

		refreshTokenUseCase := usecase.NewRefreshTokenUseCase(
			config,
			mockAuthRepo,
			mockUserService,
			mockTenantService,
			jwtSvc,
			NewMockRoleResolver(),
			NewMockPlanResolver(),
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

		// Crear refresh token válido (con tenant denormalizado, T8e)
		refreshToken := tokenMother.WithUser(userID)
		refreshToken.TenantID = tenantID
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

// T8f: la rotación single-use es check-then-act — cuando el DELETE del repo
// reporta que la fila ya no existe (otro request concurrente la consumió),
// este request NO recibe credenciales nuevas: falla con ErrInvalidToken (401),
// nunca con un 500 de infraestructura, y jamás llega a CreateRefreshToken.
func TestRefreshTokenUseCase_Execute_TokenYaConsumido(t *testing.T) {
	ctx := context.Background()
	tokenMother := authEntity.Create()

	t.Run("rotación perdida contra otro request falla con ErrInvalidToken y no emite reemplazo", func(t *testing.T) {
		// Arrange
		mockAuthRepo := repository.NewMockAuthRepository()
		mockUserService := NewMockUserService()
		mockTenantService := NewMockTenantService()

		config := usecase.AuthConfig{
			AccessTokenExpiry:  15 * time.Minute,
			RefreshTokenExpiry: 7 * 24 * time.Hour,
		}
		jwtSvc := adapter.NewJWTServiceAdapter("test-secret")

		refreshTokenUseCase := usecase.NewRefreshTokenUseCase(
			config,
			mockAuthRepo,
			mockUserService,
			mockTenantService,
			jwtSvc,
			NewMockRoleResolver(),
			NewMockPlanResolver(),
		)

		userID := uuid.New()
		tenantID := uuid.New()

		user := &port.UserData{
			ID:       userID,
			Email:    "test@example.com",
			TenantID: tenantID,
			RoleID:   uuid.New(),
			Status:   "ACTIVE",
		}
		mockUserService.SetupUser(user)

		refreshToken := tokenMother.WithUser(userID)
		refreshToken.Token = "rt-consumido-en-carrera"
		refreshToken.TenantID = tenantID
		mockAuthRepo.SetupRefreshTokens([]*entity.RefreshToken{refreshToken})

		// El repo devuelve el sentinela de T8f envuelto — como hace el repo real
		// cuando RowsAffected != 1.
		mockAuthRepo.FailMethodWith("DeleteRefreshToken",
			fmt.Errorf("%w: el DELETE tocó 0 filas", port.ErrRefreshTokenAlreadyConsumed))

		// Act
		response, err := refreshTokenUseCase.Execute(ctx, refreshToken.Token)

		// Assert
		assert.ErrorIs(t, err, usecase.ErrInvalidToken)
		assert.Nil(t, response)
		assert.Equal(t, 0, mockAuthRepo.GetCallCount("CreateRefreshToken"),
			"sin fila consumida no puede emitirse un reemplazo: sería una segunda credencial viva nacida de la misma")
	})

	t.Run("un token expirado consumido en carrera responde expirado, no 500", func(t *testing.T) {
		// Arrange
		mockAuthRepo := repository.NewMockAuthRepository()
		mockUserService := NewMockUserService()
		mockTenantService := NewMockTenantService()

		config := usecase.AuthConfig{
			AccessTokenExpiry:  15 * time.Minute,
			RefreshTokenExpiry: 7 * 24 * time.Hour,
		}
		jwtSvc := adapter.NewJWTServiceAdapter("test-secret")

		refreshTokenUseCase := usecase.NewRefreshTokenUseCase(
			config,
			mockAuthRepo,
			mockUserService,
			mockTenantService,
			jwtSvc,
			NewMockRoleResolver(),
			NewMockPlanResolver(),
		)

		userID := uuid.New()
		tenantID := uuid.New()

		refreshToken := tokenMother.WithUser(userID)
		refreshToken.Token = "rt-expirado-consumido"
		refreshToken.TenantID = tenantID
		refreshToken.ExpiresAt = time.Now().Add(-time.Hour)
		mockAuthRepo.SetupRefreshTokens([]*entity.RefreshToken{refreshToken})

		mockAuthRepo.FailMethodWith("DeleteRefreshToken",
			fmt.Errorf("%w: el DELETE tocó 0 filas", port.ErrRefreshTokenAlreadyConsumed))

		// Act
		response, err := refreshTokenUseCase.Execute(ctx, refreshToken.Token)

		// Assert
		assert.ErrorIs(t, err, usecase.ErrExpiredToken,
			"la carrera por un token expirado es igual de muerta: 401 expirado, no 500")
		assert.Nil(t, response)
	})
}
