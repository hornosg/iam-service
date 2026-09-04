package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"iam/src/access/application/usecase"
	"iam/src/access/domain/entity"
	roleMother "iam/test/access/domain/entity"
	"iam/test/access/infrastructure/persistence/repository"
)

func TestGetRoleByIDUseCase_Execute_Found_ReturnsResponse(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	getUseCase := usecase.NewGetRoleByIDUseCase(mockRepo)
	ctx := context.Background()

	mother := roleMother.Create()
	role := mother.WithName("Admin Role")
	mockRepo.SetupRoles([]*entity.Role{role})

	// Act
	resp, err := getUseCase.Execute(ctx, role.ID)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, role.ID, resp.ID)
	assert.Equal(t, "Admin Role", resp.Name)
	assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
}

func TestGetRoleByIDUseCase_Execute_NotFound_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	getUseCase := usecase.NewGetRoleByIDUseCase(mockRepo)
	ctx := context.Background()

	// Act
	resp, err := getUseCase.Execute(ctx, uuid.New())

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
}

func TestGetRoleByIDUseCase_Execute_RepoFails_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	getUseCase := usecase.NewGetRoleByIDUseCase(mockRepo)
	ctx := context.Background()

	mockRepo.ShouldFailOn("GetByID")

	// Act
	resp, err := getUseCase.Execute(ctx, uuid.New())

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, repository.ErrMockFailedOp, err)
}
