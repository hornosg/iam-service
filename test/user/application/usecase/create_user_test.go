package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"iam/src/user/application/request"
	"iam/src/user/application/usecase"
	"iam/src/user/domain/exception"
	"iam/src/user/domain/value_object"
	"iam/test/user/domain/entity"
	"iam/test/user/infrastructure/persistence/repository"
)

func TestCreateUserUseCase_Execute(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	createUseCase := usecase.NewCreateUserUseCase(mockRepo)
	ctx := context.Background()
	userMother := entity.Create()

	t.Run("debería crear un usuario con éxito", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		tenantID := uuid.New()
		roleID := uuid.New()
		req := &request.CreateUserRequest{
			Email:    "nuevo@example.com",
			Password: "password123",
			TenantID: tenantID,
			RoleID:   roleID,
			Provider: "LOCAL",
		}

		// Act
		userResponse, err := createUseCase.Execute(ctx, req)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, userResponse)
		assert.Equal(t, req.Email, userResponse.Email)
		assert.Equal(t, tenantID, userResponse.TenantID)
		assert.Equal(t, roleID, userResponse.RoleID)
		assert.Equal(t, value_object.StatusPending, userResponse.Status)
		assert.Equal(t, "LOCAL", userResponse.Provider)
		assert.NotEmpty(t, userResponse.ID)

		// Verificar llamadas al repositorio
		assert.Equal(t, 1, mockRepo.GetCallCount("ExistsByEmail"))
		assert.Equal(t, 1, mockRepo.GetCallCount("Create"))

		// Verificar que el usuario está en el repositorio
		users := mockRepo.GetUsers()
		assert.Len(t, users, 1)
		assert.Equal(t, userResponse.ID, users[0].ID)
	})

	t.Run("debería crear un usuario federado sin password", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		tenantID := uuid.New()
		roleID := uuid.New()
		req := &request.CreateUserRequest{
			Email:    "federado@example.com",
			TenantID: tenantID,
			RoleID:   roleID,
			Provider: "GOOGLE",
		}

		// Act
		userResponse, err := createUseCase.Execute(ctx, req)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, userResponse)
		assert.Equal(t, req.Email, userResponse.Email)
		assert.Equal(t, "GOOGLE", userResponse.Provider)
		assert.Equal(t, 1, mockRepo.GetCallCount("Create"))
	})

	t.Run("debería fallar si el email ya existe", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		existingUser := userMother.WithEmail("existente@example.com")
		mockRepo.SetupUsers([]*entity.User{existingUser})

		req := &request.CreateUserRequest{
			Email:    "existente@example.com",
			Password: "password123",
			TenantID: existingUser.TenantID,
			RoleID:   uuid.New(),
			Provider: "LOCAL",
		}

		// Act
		userResponse, err := createUseCase.Execute(ctx, req)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, userResponse)
		assert.Equal(t, exception.ErrUserAlreadyExists, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("ExistsByEmail"))
		assert.Equal(t, 0, mockRepo.GetCallCount("Create")) // No debe llamar a Create
	})

	t.Run("debería fallar si el request es inválido - email vacío", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		req := &request.CreateUserRequest{
			Email:    "",
			Password: "password123",
			TenantID: uuid.New(),
			RoleID:   uuid.New(),
			Provider: "LOCAL",
		}

		// Act
		userResponse, err := createUseCase.Execute(ctx, req)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, userResponse)
		assert.Contains(t, err.Error(), "email es requerido")
		assert.Equal(t, 0, mockRepo.GetCallCount("ExistsByEmail"))
		assert.Equal(t, 0, mockRepo.GetCallCount("Create"))
	})

	t.Run("debería fallar si el request es inválido - password corto", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		req := &request.CreateUserRequest{
			Email:    "test@example.com",
			Password: "123",
			TenantID: uuid.New(),
			RoleID:   uuid.New(),
			Provider: "LOCAL",
		}

		// Act
		userResponse, err := createUseCase.Execute(ctx, req)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, userResponse)
		assert.Contains(t, err.Error(), "password debe tener al menos 8 caracteres")
		assert.Equal(t, 0, mockRepo.GetCallCount("ExistsByEmail"))
		assert.Equal(t, 0, mockRepo.GetCallCount("Create"))
	})

	t.Run("debería fallar si el email es inválido", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		req := &request.CreateUserRequest{
			Email:    "email-invalido",
			Password: "password123",
			TenantID: uuid.New(),
			RoleID:   uuid.New(),
			Provider: "LOCAL",
		}

		// Act
		userResponse, err := createUseCase.Execute(ctx, req)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, userResponse)
		assert.Equal(t, exception.ErrInvalidEmail, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("ExistsByEmail"))
		assert.Equal(t, 0, mockRepo.GetCallCount("Create"))
	})

	t.Run("debería fallar si ExistsByEmail falla", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.ShouldFailOn("ExistsByEmail")

		req := &request.CreateUserRequest{
			Email:    "test@example.com",
			Password: "password123",
			TenantID: uuid.New(),
			RoleID:   uuid.New(),
			Provider: "LOCAL",
		}

		// Act
		userResponse, err := createUseCase.Execute(ctx, req)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, userResponse)
		assert.Equal(t, repository.ErrMockFailedOp, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("ExistsByEmail"))
		assert.Equal(t, 0, mockRepo.GetCallCount("Create"))
	})

	t.Run("debería fallar si Create falla", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.ShouldFailOn("Create")

		req := &request.CreateUserRequest{
			Email:    "test@example.com",
			Password: "password123",
			TenantID: uuid.New(),
			RoleID:   uuid.New(),
			Provider: "LOCAL",
		}

		// Act
		userResponse, err := createUseCase.Execute(ctx, req)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, userResponse)
		assert.Equal(t, repository.ErrMockFailedOp, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("ExistsByEmail"))
		assert.Equal(t, 1, mockRepo.GetCallCount("Create"))
	})

	t.Run("debería fallar si tenantID es nil", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		req := &request.CreateUserRequest{
			Email:    "test@example.com",
			Password: "password123",
			TenantID: uuid.Nil,
			RoleID:   uuid.New(),
			Provider: "LOCAL",
		}

		// Act
		userResponse, err := createUseCase.Execute(ctx, req)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, userResponse)
		assert.Contains(t, err.Error(), "tenant_id es requerido")
		assert.Equal(t, 0, mockRepo.GetCallCount("ExistsByEmail"))
		assert.Equal(t, 0, mockRepo.GetCallCount("Create"))
	})

	t.Run("debería fallar si roleID es nil", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		req := &request.CreateUserRequest{
			Email:    "test@example.com",
			Password: "password123",
			TenantID: uuid.New(),
			RoleID:   uuid.Nil,
			Provider: "LOCAL",
		}

		// Act
		userResponse, err := createUseCase.Execute(ctx, req)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, userResponse)
		assert.Contains(t, err.Error(), "role_id es requerido")
		assert.Equal(t, 0, mockRepo.GetCallCount("ExistsByEmail"))
		assert.Equal(t, 0, mockRepo.GetCallCount("Create"))
	})
}
