package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"iam/src/access/application/request"
	"iam/src/access/application/usecase"
	"iam/src/access/domain/entity"
	"iam/src/access/domain/exception"
	roleMother "iam/test/access/domain/entity"
	"iam/test/access/infrastructure/persistence/repository"
)

func TestUpdateRoleUseCase_Execute_HappyPath_UpdatesRole(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	updateUseCase := usecase.NewUpdateRoleUseCase(mockRepo)
	ctx := context.Background()

	mother := roleMother.Create()
	role := mother.Custom()
	mockRepo.SetupRoles([]*entity.Role{role})

	name := "Updated Role Name"
	desc := "Updated description with sufficient length"
	req := &request.UpdateRoleRequest{
		Name:        &name,
		Description: &desc,
	}

	// Act
	resp, err := updateUseCase.Execute(ctx, role.ID, req)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "Updated Role Name", resp.Name)
	assert.Equal(t, "Updated description with sufficient length", resp.Description)
	assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
	assert.Equal(t, 1, mockRepo.GetCallCount("Update"))
}

func TestUpdateRoleUseCase_Execute_SystemRoleProtection_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	updateUseCase := usecase.NewUpdateRoleUseCase(mockRepo)
	ctx := context.Background()

	mother := roleMother.Create()
	role := mother.SystemAdmin()
	mockRepo.SetupRoles([]*entity.Role{role})

	name := "Hacked Admin"
	req := &request.UpdateRoleRequest{
		Name: &name,
	}

	// Act
	resp, err := updateUseCase.Execute(ctx, role.ID, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, exception.ErrSystemRoleModification, err)
}

func TestUpdateRoleUseCase_Execute_NotFound_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	updateUseCase := usecase.NewUpdateRoleUseCase(mockRepo)
	ctx := context.Background()

	name := "Name"
	req := &request.UpdateRoleRequest{
		Name: &name,
	}

	// Act
	resp, err := updateUseCase.Execute(ctx, uuid.New(), req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestUpdateRoleUseCase_Execute_ActivateRole_Succeeds(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	updateUseCase := usecase.NewUpdateRoleUseCase(mockRepo)
	ctx := context.Background()

	mother := roleMother.Create()
	role := mother.Custom()
	role.IsActive = false
	mockRepo.SetupRoles([]*entity.Role{role})

	isActive := true
	req := &request.UpdateRoleRequest{
		IsActive: &isActive,
	}

	// Act
	resp, err := updateUseCase.Execute(ctx, role.ID, req)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.IsActive)
}

func TestUpdateRoleUseCase_Execute_DeactivateRole_Succeeds(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	updateUseCase := usecase.NewUpdateRoleUseCase(mockRepo)
	ctx := context.Background()

	mother := roleMother.Create()
	role := mother.Custom()
	mockRepo.SetupRoles([]*entity.Role{role})

	isActive := false
	req := &request.UpdateRoleRequest{
		IsActive: &isActive,
	}

	// Act
	resp, err := updateUseCase.Execute(ctx, role.ID, req)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.False(t, resp.IsActive)
}

func TestUpdateRoleUseCase_Execute_UpdatePermissions_Succeeds(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	updateUseCase := usecase.NewUpdateRoleUseCase(mockRepo)
	ctx := context.Background()

	mother := roleMother.Create()
	role := mother.Custom()
	mockRepo.SetupRoles([]*entity.Role{role})

	newPermissions := []string{"new:perm1", "new:perm2", "new:perm3"}
	req := &request.UpdateRoleRequest{
		Permissions: &newPermissions,
	}

	// Act
	resp, err := updateUseCase.Execute(ctx, role.ID, req)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Permissions, 3)
	assert.Contains(t, resp.Permissions, "new:perm1")
}
