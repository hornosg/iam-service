package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"iam/src/access/application/usecase"
	"iam/src/access/domain/entity"
	"iam/src/access/domain/exception"
	roleMother "iam/test/access/domain/entity"
	"iam/test/access/infrastructure/persistence/repository"
)

func TestDeleteRoleUseCase_Execute_CustomRole_Succeeds(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	deleteUseCase := usecase.NewDeleteRoleUseCase(mockRepo)
	ctx := context.Background()

	mother := roleMother.Create()
	role := mother.Custom()
	mockRepo.SetupRoles([]*entity.Role{role})

	// Act
	err := deleteUseCase.Execute(ctx, role.ID)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
	assert.Equal(t, 1, mockRepo.GetCallCount("Update"))
}

func TestDeleteRoleUseCase_Execute_SystemAdminRole_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	deleteUseCase := usecase.NewDeleteRoleUseCase(mockRepo)
	ctx := context.Background()

	mother := roleMother.Create()
	role := mother.SystemAdmin()
	mockRepo.SetupRoles([]*entity.Role{role})

	// Act
	err := deleteUseCase.Execute(ctx, role.ID)

	// Assert
	assert.Error(t, err)
	assert.Equal(t, exception.ErrCannotDeleteRole, err)
}

func TestDeleteRoleUseCase_Execute_TenantAdminRole_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	deleteUseCase := usecase.NewDeleteRoleUseCase(mockRepo)
	ctx := context.Background()

	mother := roleMother.Create()
	role := mother.TenantAdmin()
	mockRepo.SetupRoles([]*entity.Role{role})

	// Act
	err := deleteUseCase.Execute(ctx, role.ID)

	// Assert
	assert.Error(t, err)
	assert.Equal(t, exception.ErrCannotDeleteRole, err)
}

func TestDeleteRoleUseCase_Execute_UserRole_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	deleteUseCase := usecase.NewDeleteRoleUseCase(mockRepo)
	ctx := context.Background()

	mother := roleMother.Create()
	role := mother.User()
	mockRepo.SetupRoles([]*entity.Role{role})

	// Act
	err := deleteUseCase.Execute(ctx, role.ID)

	// Assert
	assert.Error(t, err)
	assert.Equal(t, exception.ErrCannotDeleteRole, err)
}

func TestDeleteRoleUseCase_Execute_ReadOnlyRole_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	deleteUseCase := usecase.NewDeleteRoleUseCase(mockRepo)
	ctx := context.Background()

	mother := roleMother.Create()
	role := mother.ReadOnly()
	mockRepo.SetupRoles([]*entity.Role{role})

	// Act
	err := deleteUseCase.Execute(ctx, role.ID)

	// Assert
	assert.Error(t, err)
	assert.Equal(t, exception.ErrCannotDeleteRole, err)
}

func TestDeleteRoleUseCase_Execute_NotFound_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	deleteUseCase := usecase.NewDeleteRoleUseCase(mockRepo)
	ctx := context.Background()

	// Act
	err := deleteUseCase.Execute(ctx, uuid.New())

	// Assert
	assert.Error(t, err)
}

func TestDeleteRoleUseCase_Execute_UpdateFails_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	deleteUseCase := usecase.NewDeleteRoleUseCase(mockRepo)
	ctx := context.Background()

	mother := roleMother.Create()
	role := mother.Custom()
	mockRepo.SetupRoles([]*entity.Role{role})
	mockRepo.ShouldFailOn("Update")

	// Act
	err := deleteUseCase.Execute(ctx, role.ID)

	// Assert
	assert.Error(t, err)
	assert.Equal(t, repository.ErrMockFailedOp, err)
}
