package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"iam/src/identity/application/usecase"
	"iam/src/identity/domain/entity"
	"iam/src/identity/domain/exception"
	userMother "iam/test/identity/user/domain/entity"
	"iam/test/identity/user/infrastructure/persistence/repository"
)

func TestGetUserByIDUseCase_Execute_UserFound_ReturnsResponse(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	getUseCase := usecase.NewGetUserByIDUseCase(mockRepo)
	ctx := context.Background()

	mother := userMother.Create()
	user := mother.WithEmail("found@example.com")
	mockRepo.SetupUsers([]*entity.User{user})

	// Act
	resp, err := getUseCase.Execute(ctx, user.ID)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, user.ID, resp.ID)
	assert.Equal(t, "found@example.com", resp.Email)
	assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
}

func TestGetUserByIDUseCase_Execute_UserNotFound_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	getUseCase := usecase.NewGetUserByIDUseCase(mockRepo)
	ctx := context.Background()

	// Act
	resp, err := getUseCase.Execute(ctx, uuid.New())

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, exception.ErrUserNotFound, err)
	assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
}

func TestGetUserByIDUseCase_Execute_RepoFails_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	getUseCase := usecase.NewGetUserByIDUseCase(mockRepo)
	ctx := context.Background()

	mockRepo.ShouldFailOn("GetByID")

	// Act
	resp, err := getUseCase.Execute(ctx, uuid.New())

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
}
