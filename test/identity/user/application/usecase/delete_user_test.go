package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"iam/src/identity/application/usecase"
	"iam/src/identity/domain/entity"
	"iam/src/identity/domain/exception"
	userMother "iam/test/identity/user/domain/entity"
	"iam/test/identity/user/infrastructure/persistence/repository"
)

func TestDeleteUserUseCase_Execute_HappyPath_DeletesUser(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	deleteUseCase := usecase.NewDeleteUserUseCase(mockRepo)
	ctx := context.Background()

	mother := userMother.Create()
	user := mother.WithDefaults()
	mockRepo.SetupUsers([]*entity.User{user})

	// Act
	err := deleteUseCase.Execute(ctx, user.ID)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
	assert.Equal(t, 1, mockRepo.GetCallCount("Delete"))
}

func TestDeleteUserUseCase_Execute_UserNotFound_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	deleteUseCase := usecase.NewDeleteUserUseCase(mockRepo)
	ctx := context.Background()

	// Act
	err := deleteUseCase.Execute(ctx, uuid.New())

	// Assert
	assert.Error(t, err)
	assert.Equal(t, exception.ErrUserNotFound, err)
	assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
	assert.Equal(t, 0, mockRepo.GetCallCount("Delete"))
}

func TestDeleteUserUseCase_Execute_DeleteFails_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	deleteUseCase := usecase.NewDeleteUserUseCase(mockRepo)
	ctx := context.Background()

	mother := userMother.Create()
	user := mother.WithDefaults()
	mockRepo.SetupUsers([]*entity.User{user})
	mockRepo.ShouldFailOn("Delete")

	// Act
	err := deleteUseCase.Execute(ctx, user.ID)

	// Assert
	assert.Error(t, err)
	assert.Equal(t, repository.ErrMockFailedOp, err)
}
