package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"iam/src/user/application/usecase"
	srcEntity "iam/src/user/domain/entity"
	"iam/test/user/domain/entity"
	"iam/test/user/infrastructure/persistence/repository"
)

func TestUserFinderUseCase_FindUserByEmail_ReturnsUser(t *testing.T) {
	mockRepo := repository.NewMockUserRepository()
	uc := usecase.NewUserFinderUseCase(mockRepo)
	ctx := context.Background()
	mother := entity.Create()

	t.Run("debería encontrar usuario por email sin tenant_id", func(t *testing.T) {
		user := mother.WithDefaults()
		mockRepo.SetupUsers([]*srcEntity.User{user})
		mockRepo.ResetCallHistory()

		data, err := uc.FindUserByEmail(ctx, user.Email.Value(), nil)

		require.NoError(t, err)
		require.NotNil(t, data)
		assert.Equal(t, user.ID, data.ID)
		assert.Equal(t, user.Email.Value(), data.Email)
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByEmail"))
	})

	t.Run("debería encontrar usuario por email con tenant_id", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		user := mother.WithDefaults()
		mockRepo.SetupUsers([]*srcEntity.User{user})

		tenantID := user.TenantID
		data, err := uc.FindUserByEmail(ctx, user.Email.Value(), &tenantID)

		require.NoError(t, err)
		require.NotNil(t, data)
		assert.Equal(t, user.ID, data.ID)
	})

	t.Run("debería retornar error si el usuario no existe", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.SetupUsers(nil)

		data, err := uc.FindUserByEmail(ctx, "noexiste@test.com", nil)

		assert.Error(t, err)
		assert.Nil(t, data)
	})

	t.Run("debería retornar error si el repositorio falla", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.ShouldFailOn("GetByEmail")

		data, err := uc.FindUserByEmail(ctx, "test@test.com", nil)

		assert.Error(t, err)
		assert.Nil(t, data)
		assert.Equal(t, repository.ErrMockFailedOp, err)
	})
}

func TestUserFinderUseCase_FindUserByID_ReturnsUser(t *testing.T) {
	mockRepo := repository.NewMockUserRepository()
	uc := usecase.NewUserFinderUseCase(mockRepo)
	ctx := context.Background()
	mother := entity.Create()

	t.Run("debería encontrar usuario por ID", func(t *testing.T) {
		user := mother.WithDefaults()
		mockRepo.SetupUsers([]*srcEntity.User{user})
		mockRepo.ResetCallHistory()

		data, err := uc.FindUserByID(ctx, user.ID)

		require.NoError(t, err)
		require.NotNil(t, data)
		assert.Equal(t, user.ID, data.ID)
		assert.Equal(t, user.Email.Value(), data.Email)
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
	})

	t.Run("debería retornar error si el usuario no existe", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.SetupUsers(nil)

		data, err := uc.FindUserByID(ctx, uuid.New())

		assert.Error(t, err)
		assert.Nil(t, data)
	})

	t.Run("debería retornar error si el repositorio falla", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.ShouldFailOn("GetByID")

		data, err := uc.FindUserByID(ctx, uuid.New())

		assert.Error(t, err)
		assert.Nil(t, data)
		assert.Equal(t, repository.ErrMockFailedOp, err)
	})
}
