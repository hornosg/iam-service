package usecase_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"iam/src/role/application/request"
	"iam/src/role/application/usecase"
	"iam/src/role/domain/entity"
	"iam/src/role/domain/exception"
	roleMother "iam/test/role/domain/entity"
	"iam/test/role/infrastructure/persistence/repository"
)

func TestCreateRoleUseCase_Execute_HappyPath_CreatesRole(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	createUseCase := usecase.NewCreateRoleUseCase(mockRepo)
	ctx := context.Background()

	req := &request.CreateRoleRequest{
		Name:        "Manager",
		Description: "Rol de gerente con permisos especiales",
		Type:        "CUSTOM",
		Permissions: []string{"read:products", "write:products"},
	}

	// Act
	resp, err := createUseCase.Execute(ctx, req)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "Manager", resp.Name)
	assert.Equal(t, "CUSTOM", resp.Type)
	assert.True(t, resp.IsActive)
	assert.Len(t, resp.Permissions, 2)
	assert.Contains(t, resp.Permissions, "read:products")
	assert.Contains(t, resp.Permissions, "write:products")
	assert.Equal(t, 1, mockRepo.GetCallCount("ExistsByName"))
	assert.Equal(t, 1, mockRepo.GetCallCount("Create"))
}

func TestCreateRoleUseCase_Execute_SystemRole_NilTenantID(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	createUseCase := usecase.NewCreateRoleUseCase(mockRepo)
	ctx := context.Background()

	req := &request.CreateRoleRequest{
		Name:        "System Admin",
		Description: "Administrador del sistema completo",
		Type:        "SYSTEM_ADMIN",
		Permissions: []string{"system:admin"},
	}

	// Act
	resp, err := createUseCase.Execute(ctx, req)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "SYSTEM_ADMIN", resp.Type)
	assert.True(t, resp.IsSystem)
	assert.False(t, resp.IsTenant)
}

func TestCreateRoleUseCase_Execute_DuplicateName_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	createUseCase := usecase.NewCreateRoleUseCase(mockRepo)
	ctx := context.Background()

	mother := roleMother.Create()
	existing := mother.WithName("Existing Role")
	// El rol existente es de sistema (nil tenantID)
	existing.TenantID = nil
	mockRepo.SetupRoles([]*entity.Role{existing})

	req := &request.CreateRoleRequest{
		Name:        "Existing Role",
		Description: "Intento de duplicado de rol",
		Type:        "CUSTOM",
	}

	// Act
	resp, err := createUseCase.Execute(ctx, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, exception.ErrRoleAlreadyExists, err)
}

func TestCreateRoleUseCase_Execute_InvalidType_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	createUseCase := usecase.NewCreateRoleUseCase(mockRepo)
	ctx := context.Background()

	req := &request.CreateRoleRequest{
		Name:        "Invalid Role",
		Description: "Rol con tipo invalido para test",
		Type:        "INVALID_TYPE",
	}

	// Act
	resp, err := createUseCase.Execute(ctx, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, exception.ErrInvalidRoleType, err)
}

func TestCreateRoleUseCase_Execute_RepoFails_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockRoleRepository()
	createUseCase := usecase.NewCreateRoleUseCase(mockRepo)
	ctx := context.Background()

	mockRepo.ShouldFailOn("Create")

	req := &request.CreateRoleRequest{
		Name:        "Role",
		Description: "Descripcion del rol de prueba",
		Type:        "CUSTOM",
	}

	// Act
	resp, err := createUseCase.Execute(ctx, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, repository.ErrMockFailedOp, err)
}
