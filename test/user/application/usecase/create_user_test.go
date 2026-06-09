package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"iam/src/user/application/request"
	"iam/src/user/application/usecase"
	"iam/src/user/domain/entity"
	"iam/src/user/domain/exception"
	"iam/src/user/domain/value_object"
	userMother "iam/test/user/domain/entity"
	"iam/test/user/infrastructure/persistence/repository"
)

func TestCreateUserUseCase_Execute_HappyPath_CreatesUser(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	createUseCase := usecase.NewCreateUserUseCase(mockRepo)
	ctx := context.Background()

	tenantID := uuid.New()
	roleID := uuid.New()

	req := &request.CreateUserRequest{
		Email:    "newuser@example.com",
		Password: "securepassword123",
		TenantID: tenantID,
		RoleID:   roleID,
	}

	// Act
	resp, err := createUseCase.Execute(ctx, req)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "newuser@example.com", resp.Email)
	assert.Equal(t, tenantID, resp.TenantID)
	assert.Equal(t, roleID, resp.RoleID)
	assert.Equal(t, value_object.StatusPending, resp.Status)
	assert.Equal(t, "LOCAL", resp.Provider)
	assert.NotEqual(t, uuid.Nil, resp.ID)
	assert.Equal(t, 1, mockRepo.GetCallCount("ExistsByEmail"))
	assert.Equal(t, 1, mockRepo.GetCallCount("Create"))
}

func TestCreateUserUseCase_Execute_DuplicateEmail_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	createUseCase := usecase.NewCreateUserUseCase(mockRepo)
	ctx := context.Background()

	mother := userMother.Create()
	existingUser := mother.WithEmail("existing@example.com")
	mockRepo.SetupUsers([]*entity.User{existingUser})

	tenantID := existingUser.TenantID
	roleID := uuid.New()

	req := &request.CreateUserRequest{
		Email:    "existing@example.com",
		Password: "securepassword123",
		TenantID: tenantID,
		RoleID:   roleID,
	}

	// Act
	resp, err := createUseCase.Execute(ctx, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, exception.ErrUserAlreadyExists, err)
	assert.Equal(t, 1, mockRepo.GetCallCount("ExistsByEmail"))
	assert.Equal(t, 0, mockRepo.GetCallCount("Create"))
}

func TestCreateUserUseCase_Execute_InvalidEmail_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	createUseCase := usecase.NewCreateUserUseCase(mockRepo)
	ctx := context.Background()

	req := &request.CreateUserRequest{
		Email:    "invalid-email",
		Password: "securepassword123",
		TenantID: uuid.New(),
		RoleID:   uuid.New(),
	}

	// Act
	resp, err := createUseCase.Execute(ctx, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestCreateUserUseCase_Execute_MissingPassword_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	createUseCase := usecase.NewCreateUserUseCase(mockRepo)
	ctx := context.Background()

	req := &request.CreateUserRequest{
		Email:    "test@example.com",
		Password: "",
		TenantID: uuid.New(),
		RoleID:   uuid.New(),
	}

	// Act
	resp, err := createUseCase.Execute(ctx, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestCreateUserUseCase_Execute_ShortPassword_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	createUseCase := usecase.NewCreateUserUseCase(mockRepo)
	ctx := context.Background()

	req := &request.CreateUserRequest{
		Email:    "test@example.com",
		Password: "short",
		TenantID: uuid.New(),
		RoleID:   uuid.New(),
	}

	// Act
	resp, err := createUseCase.Execute(ctx, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestCreateUserUseCase_Execute_RepoFails_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	createUseCase := usecase.NewCreateUserUseCase(mockRepo)
	ctx := context.Background()

	mockRepo.ShouldFailOn("Create")

	req := &request.CreateUserRequest{
		Email:    "test@example.com",
		Password: "securepassword123",
		TenantID: uuid.New(),
		RoleID:   uuid.New(),
	}

	// Act
	resp, err := createUseCase.Execute(ctx, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, repository.ErrMockFailedOp, err)
}
