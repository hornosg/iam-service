package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hornosg/iam-service/src/identity/application/request"
	"github.com/hornosg/iam-service/src/identity/application/usecase"
	"github.com/hornosg/iam-service/src/identity/domain/entity"
	"github.com/hornosg/iam-service/src/identity/domain/exception"
	"github.com/hornosg/iam-service/src/identity/domain/value_object"
	userMother "github.com/hornosg/iam-service/test/identity/user/domain/entity"
	"github.com/hornosg/iam-service/test/identity/user/infrastructure/persistence/repository"
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
