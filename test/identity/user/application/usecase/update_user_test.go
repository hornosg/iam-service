package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"iam/src/identity/application/request"
	"iam/src/identity/application/usecase"
	"iam/src/identity/domain/entity"
	"iam/src/identity/domain/exception"
	"iam/src/identity/domain/value_object"
	userMother "iam/test/identity/user/domain/entity"
	"iam/test/identity/user/infrastructure/persistence/repository"
)

func TestUpdateUserUseCase_Execute_UpdateEmail_Succeeds(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	updateUseCase := usecase.NewUpdateUserUseCase(mockRepo)
	ctx := context.Background()

	mother := userMother.Create()
	user := mother.WithEmail("old@example.com")
	mockRepo.SetupUsers([]*entity.User{user})

	newEmail := "new@example.com"
	req := &request.UpdateUserRequest{
		ID:    user.ID,
		Email: &newEmail,
	}

	// Act
	resp, err := updateUseCase.Execute(ctx, req)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "new@example.com", resp.Email)
	assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
	assert.Equal(t, 1, mockRepo.GetCallCount("ExistsByEmail"))
	assert.Equal(t, 1, mockRepo.GetCallCount("Update"))
}

func TestUpdateUserUseCase_Execute_UpdateStatus_Succeeds(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	updateUseCase := usecase.NewUpdateUserUseCase(mockRepo)
	ctx := context.Background()

	mother := userMother.Create()
	user := mother.WithDefaults()
	mockRepo.SetupUsers([]*entity.User{user})

	status := value_object.StatusActive
	req := &request.UpdateUserRequest{
		ID:     user.ID,
		Status: &status,
	}

	// Act
	resp, err := updateUseCase.Execute(ctx, req)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, value_object.StatusActive, resp.Status)
}

func TestUpdateUserUseCase_Execute_UserNotFound_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	updateUseCase := usecase.NewUpdateUserUseCase(mockRepo)
	ctx := context.Background()

	newEmail := "new@example.com"
	req := &request.UpdateUserRequest{
		ID:    uuid.New(),
		Email: &newEmail,
	}

	// Act
	resp, err := updateUseCase.Execute(ctx, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, exception.ErrUserNotFound, err)
}

func TestUpdateUserUseCase_Execute_DuplicateEmail_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	updateUseCase := usecase.NewUpdateUserUseCase(mockRepo)
	ctx := context.Background()

	mother := userMother.Create()
	tenantID := uuid.New()
	user1 := mother.WithEmail("user1@example.com")
	user1.TenantID = tenantID
	user2 := mother.WithEmail("user2@example.com")
	user2.TenantID = tenantID
	mockRepo.SetupUsers([]*entity.User{user1, user2})

	existingEmail := "user2@example.com"
	req := &request.UpdateUserRequest{
		ID:    user1.ID,
		Email: &existingEmail,
	}

	// Act
	resp, err := updateUseCase.Execute(ctx, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, exception.ErrUserAlreadyExists, err)
}

func TestUpdateUserUseCase_Execute_InvalidEmail_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	updateUseCase := usecase.NewUpdateUserUseCase(mockRepo)
	ctx := context.Background()

	mother := userMother.Create()
	user := mother.WithDefaults()
	mockRepo.SetupUsers([]*entity.User{user})

	invalidEmail := "not-an-email"
	req := &request.UpdateUserRequest{
		ID:    user.ID,
		Email: &invalidEmail,
	}

	// Act
	resp, err := updateUseCase.Execute(ctx, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, exception.ErrInvalidEmail, err)
}

func TestUpdateUserUseCase_Execute_UpdateRole_Succeeds(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	updateUseCase := usecase.NewUpdateUserUseCase(mockRepo)
	ctx := context.Background()

	mother := userMother.Create()
	user := mother.WithDefaults()
	mockRepo.SetupUsers([]*entity.User{user})

	newRoleID := uuid.New()
	req := &request.UpdateUserRequest{
		ID:     user.ID,
		RoleID: &newRoleID,
	}

	// Act
	resp, err := updateUseCase.Execute(ctx, req)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, newRoleID, resp.RoleID)
}
