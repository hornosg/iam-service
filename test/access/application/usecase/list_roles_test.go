package usecase_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hornosg/iam-service/src/access/application/usecase"
	"github.com/hornosg/iam-service/src/access/domain/entity"
	roleMother "github.com/hornosg/iam-service/test/access/domain/entity"
	"github.com/hornosg/iam-service/test/access/infrastructure/persistence/repository"
)

func TestListRolesUseCase_Execute_ReturnsRoles(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	listUseCase := usecase.NewListRolesUseCase(mockRepo)
	ctx := context.Background()

	mother := roleMother.Create()
	role1 := mother.SystemAdmin()
	role2 := mother.Custom()
	mockRepo.SetupRoles([]*entity.Role{role1, role2})

	// Act
	resp, err := listUseCase.Execute(ctx, 1, 10)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 2, resp.TotalCount)
	assert.Len(t, resp.Roles, 2)
	assert.Equal(t, 1, resp.Page)
	assert.Equal(t, 10, resp.PageSize)
	assert.Equal(t, 1, mockRepo.GetCallCount("List"))
	assert.Equal(t, 1, mockRepo.GetCallCount("Count"))
}

func TestListRolesUseCase_Execute_EmptyResult(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	listUseCase := usecase.NewListRolesUseCase(mockRepo)
	ctx := context.Background()

	// Act
	resp, err := listUseCase.Execute(ctx, 1, 10)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 0, resp.TotalCount)
	assert.Empty(t, resp.Roles)
}

func TestListRolesUseCase_GetSystemRoles_ReturnsGlobalCatalog(t *testing.T) {
	// ACC-E02 T10/T11: `roles` es catálogo global; GetSystemRoles devuelve
	// todos los roles (mismo comportamiento que el repositorio Postgres).
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	listUseCase := usecase.NewListRolesUseCase(mockRepo)
	ctx := context.Background()

	mother := roleMother.Create()
	sysRole := mother.SystemAdmin()
	otherRole := mother.Custom()
	mockRepo.SetupRoles([]*entity.Role{sysRole, otherRole})

	// Act
	resp, err := listUseCase.GetSystemRoles(ctx)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 1, mockRepo.GetCallCount("GetSystemRoles"))
	assert.Len(t, resp.Roles, 2)
}

func TestListRolesUseCase_Execute_ListFails_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	listUseCase := usecase.NewListRolesUseCase(mockRepo)
	ctx := context.Background()

	mockRepo.ShouldFailOn("List")

	// Act
	resp, err := listUseCase.Execute(ctx, 1, 10)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, repository.ErrMockFailedOp, err)
}

func TestListRolesUseCase_Execute_CountFails_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	listUseCase := usecase.NewListRolesUseCase(mockRepo)
	ctx := context.Background()

	mockRepo.ShouldFailOn("Count")

	// Act
	resp, err := listUseCase.Execute(ctx, 1, 10)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
}
