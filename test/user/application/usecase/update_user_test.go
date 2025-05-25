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

func TestUpdateUserUseCase_Execute(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	updateUseCase := usecase.NewUpdateUserUseCase(mockRepo)
	ctx := context.Background()
	userMother := entity.Create()

	t.Run("debería actualizar el email del usuario con éxito", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		existingUser := userMother.WithEmail("original@example.com")
		mockRepo.SetupUsers([]*entity.User{existingUser})

		newEmail := "nuevo@example.com"
		req := &request.UpdateUserRequest{
			ID:    existingUser.ID,
			Email: &newEmail,
		}

		// Act
		userResponse, err := updateUseCase.Execute(ctx, req)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, userResponse)
		assert.Equal(t, newEmail, userResponse.Email)
		assert.Equal(t, existingUser.ID, userResponse.ID)

		// Verificar llamadas al repositorio
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
		assert.Equal(t, 1, mockRepo.GetCallCount("ExistsByEmail"))
		assert.Equal(t, 1, mockRepo.GetCallCount("Update"))
	})

	t.Run("debería actualizar el rol del usuario con éxito", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		existingUser := userMother.WithDefaults()
		mockRepo.SetupUsers([]*entity.User{existingUser})

		newRoleID := uuid.New()
		req := &request.UpdateUserRequest{
			ID:     existingUser.ID,
			RoleID: &newRoleID,
		}

		// Act
		userResponse, err := updateUseCase.Execute(ctx, req)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, userResponse)
		assert.Equal(t, newRoleID, userResponse.RoleID)
		assert.Equal(t, existingUser.ID, userResponse.ID)

		// Verificar llamadas al repositorio
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
		assert.Equal(t, 0, mockRepo.GetCallCount("ExistsByEmail")) // No debe verificar email
		assert.Equal(t, 1, mockRepo.GetCallCount("Update"))
	})

	t.Run("debería actualizar el status del usuario con éxito", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		existingUser := userMother.Pending()
		mockRepo.SetupUsers([]*entity.User{existingUser})

		newStatus := value_object.StatusActive
		req := &request.UpdateUserRequest{
			ID:     existingUser.ID,
			Status: &newStatus,
		}

		// Act
		userResponse, err := updateUseCase.Execute(ctx, req)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, userResponse)
		assert.Equal(t, newStatus, userResponse.Status)
		assert.Equal(t, existingUser.ID, userResponse.ID)

		// Verificar llamadas al repositorio
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
		assert.Equal(t, 0, mockRepo.GetCallCount("ExistsByEmail"))
		assert.Equal(t, 1, mockRepo.GetCallCount("Update"))
	})

	t.Run("debería actualizar múltiples campos con éxito", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		existingUser := userMother.WithEmail("original@example.com").Pending()
		mockRepo.SetupUsers([]*entity.User{existingUser})

		newEmail := "nuevo@example.com"
		newRoleID := uuid.New()
		newStatus := value_object.StatusActive
		req := &request.UpdateUserRequest{
			ID:     existingUser.ID,
			Email:  &newEmail,
			RoleID: &newRoleID,
			Status: &newStatus,
		}

		// Act
		userResponse, err := updateUseCase.Execute(ctx, req)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, userResponse)
		assert.Equal(t, newEmail, userResponse.Email)
		assert.Equal(t, newRoleID, userResponse.RoleID)
		assert.Equal(t, newStatus, userResponse.Status)

		// Verificar llamadas al repositorio
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
		assert.Equal(t, 1, mockRepo.GetCallCount("ExistsByEmail"))
		assert.Equal(t, 1, mockRepo.GetCallCount("Update"))
	})

	t.Run("debería fallar si el usuario no existe", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		nonExistentID := uuid.New()
		newEmail := "nuevo@example.com"
		req := &request.UpdateUserRequest{
			ID:    nonExistentID,
			Email: &newEmail,
		}

		// Act
		userResponse, err := updateUseCase.Execute(ctx, req)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, userResponse)
		assert.Equal(t, exception.ErrUserNotFound, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
		assert.Equal(t, 0, mockRepo.GetCallCount("ExistsByEmail"))
		assert.Equal(t, 0, mockRepo.GetCallCount("Update"))
	})

	t.Run("debería fallar si el nuevo email ya está en uso", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		existingUser1 := userMother.WithEmail("usuario1@example.com")
		existingUser2 := userMother.WithEmail("usuario2@example.com")
		mockRepo.SetupUsers([]*entity.User{existingUser1, existingUser2})

		// Intentar cambiar el email del usuario1 al email del usuario2
		emailEnUso := "usuario2@example.com"
		req := &request.UpdateUserRequest{
			ID:    existingUser1.ID,
			Email: &emailEnUso,
		}

		// Act
		userResponse, err := updateUseCase.Execute(ctx, req)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, userResponse)
		assert.Equal(t, exception.ErrUserAlreadyExists, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
		assert.Equal(t, 1, mockRepo.GetCallCount("ExistsByEmail"))
		assert.Equal(t, 0, mockRepo.GetCallCount("Update"))
	})

	t.Run("debería fallar si el email es inválido", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		existingUser := userMother.WithDefaults()
		mockRepo.SetupUsers([]*entity.User{existingUser})

		emailInvalido := "email-invalido"
		req := &request.UpdateUserRequest{
			ID:    existingUser.ID,
			Email: &emailInvalido,
		}

		// Act
		userResponse, err := updateUseCase.Execute(ctx, req)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, userResponse)
		assert.Equal(t, exception.ErrInvalidEmail, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
		assert.Equal(t, 0, mockRepo.GetCallCount("ExistsByEmail"))
		assert.Equal(t, 0, mockRepo.GetCallCount("Update"))
	})

	t.Run("debería fallar si GetByID falla", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.ShouldFailOn("GetByID")

		userID := uuid.New()
		newEmail := "nuevo@example.com"
		req := &request.UpdateUserRequest{
			ID:    userID,
			Email: &newEmail,
		}

		// Act
		userResponse, err := updateUseCase.Execute(ctx, req)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, userResponse)
		assert.Equal(t, exception.ErrUserNotFound, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
		assert.Equal(t, 0, mockRepo.GetCallCount("ExistsByEmail"))
		assert.Equal(t, 0, mockRepo.GetCallCount("Update"))
	})

	t.Run("debería fallar si ExistsByEmail falla", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		existingUser := userMother.WithDefaults()
		mockRepo.SetupUsers([]*entity.User{existingUser})
		mockRepo.ShouldFailOn("ExistsByEmail")

		newEmail := "nuevo@example.com"
		req := &request.UpdateUserRequest{
			ID:    existingUser.ID,
			Email: &newEmail,
		}

		// Act
		userResponse, err := updateUseCase.Execute(ctx, req)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, userResponse)
		assert.Equal(t, repository.ErrMockFailedOp, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
		assert.Equal(t, 1, mockRepo.GetCallCount("ExistsByEmail"))
		assert.Equal(t, 0, mockRepo.GetCallCount("Update"))
	})

	t.Run("debería fallar si Update falla", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		existingUser := userMother.WithDefaults()
		mockRepo.SetupUsers([]*entity.User{existingUser})
		mockRepo.ShouldFailOn("Update")

		newEmail := "nuevo@example.com"
		req := &request.UpdateUserRequest{
			ID:    existingUser.ID,
			Email: &newEmail,
		}

		// Act
		userResponse, err := updateUseCase.Execute(ctx, req)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, userResponse)
		assert.Equal(t, repository.ErrMockFailedOp, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
		assert.Equal(t, 1, mockRepo.GetCallCount("ExistsByEmail"))
		assert.Equal(t, 1, mockRepo.GetCallCount("Update"))
	})

	t.Run("no debería hacer nada si no hay campos para actualizar", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		existingUser := userMother.WithDefaults()
		mockRepo.SetupUsers([]*entity.User{existingUser})

		req := &request.UpdateUserRequest{
			ID: existingUser.ID,
			// No se especifica ningún campo para actualizar
		}

		// Act
		userResponse, err := updateUseCase.Execute(ctx, req)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, userResponse)
		assert.Equal(t, existingUser.ID, userResponse.ID)
		assert.Equal(t, existingUser.Email.Value(), userResponse.Email)

		// Verificar llamadas al repositorio
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
		assert.Equal(t, 0, mockRepo.GetCallCount("ExistsByEmail"))
		assert.Equal(t, 1, mockRepo.GetCallCount("Update")) // Siempre se llama a Update
	})
}
