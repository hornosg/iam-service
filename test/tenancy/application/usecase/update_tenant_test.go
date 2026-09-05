package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hornosg/iam-service/src/tenancy/application/request"
	"github.com/hornosg/iam-service/src/tenancy/application/usecase"
	"github.com/hornosg/iam-service/src/tenancy/domain/entity"
	"github.com/hornosg/iam-service/src/tenancy/domain/exception"
	tenantMother "github.com/hornosg/iam-service/test/tenancy/domain/entity"
	"github.com/hornosg/iam-service/test/tenancy/infrastructure/persistence/repository"
)

func TestUpdateTenantUseCase_Execute_HappyPath_UpdatesDetails(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockTenantRepository()
	updateUseCase := usecase.NewUpdateTenantUseCase(mockRepo)
	ctx := context.Background()

	mother := tenantMother.Create()
	tenant := mother.WithDefaults()
	mockRepo.SetupTenants([]*entity.Tenant{tenant})

	name := "Nombre Actualizado"
	desc := "Descripcion actualizada"
	req := &request.UpdateTenantRequest{
		Name:        &name,
		Description: &desc,
	}

	// Act
	resp, err := updateUseCase.Execute(ctx, tenant.ID, req)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "Nombre Actualizado", resp.Name)
	assert.Equal(t, "Descripcion actualizada", resp.Description)
	assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
	assert.Equal(t, 1, mockRepo.GetCallCount("Update"))
}

func TestUpdateTenantUseCase_Execute_NotFound_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockTenantRepository()
	updateUseCase := usecase.NewUpdateTenantUseCase(mockRepo)
	ctx := context.Background()

	name := "Nuevo Nombre"
	req := &request.UpdateTenantRequest{
		Name: &name,
	}

	// Act
	resp, err := updateUseCase.Execute(ctx, uuid.New(), req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestUpdateTenantUseCase_Execute_DeletedTenant_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockTenantRepository()
	updateUseCase := usecase.NewUpdateTenantUseCase(mockRepo)
	ctx := context.Background()

	mother := tenantMother.Create()
	tenant := mother.Deleted()
	mockRepo.SetupTenants([]*entity.Tenant{tenant})

	name := "Nombre"
	req := &request.UpdateTenantRequest{
		Name: &name,
	}

	// Act
	resp, err := updateUseCase.Execute(ctx, tenant.ID, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, exception.ErrTenantDeleted, err)
}

func TestUpdateTenantUseCase_Execute_UpdateStatus_Succeeds(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockTenantRepository()
	updateUseCase := usecase.NewUpdateTenantUseCase(mockRepo)
	ctx := context.Background()

	mother := tenantMother.Create()
	tenant := mother.WithDefaults()
	mockRepo.SetupTenants([]*entity.Tenant{tenant})

	status := "SUSPENDED"
	req := &request.UpdateTenantRequest{
		Status: &status,
	}

	// Act
	resp, err := updateUseCase.Execute(ctx, tenant.ID, req)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "SUSPENDED", resp.Status)
}

func TestUpdateTenantUseCase_Execute_InvalidStatus_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockTenantRepository()
	updateUseCase := usecase.NewUpdateTenantUseCase(mockRepo)
	ctx := context.Background()

	mother := tenantMother.Create()
	tenant := mother.WithDefaults()
	mockRepo.SetupTenants([]*entity.Tenant{tenant})

	status := "INVALID_STATUS"
	req := &request.UpdateTenantRequest{
		Status: &status,
	}

	// Act
	resp, err := updateUseCase.Execute(ctx, tenant.ID, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, exception.ErrInvalidTenantStatus, err)
}

func TestUpdateTenantUseCase_Execute_UpdateMaxUsers_Succeeds(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockTenantRepository()
	updateUseCase := usecase.NewUpdateTenantUseCase(mockRepo)
	ctx := context.Background()

	mother := tenantMother.Create()
	tenant := mother.WithDefaults()
	mockRepo.SetupTenants([]*entity.Tenant{tenant})

	maxUsers := 50
	req := &request.UpdateTenantRequest{
		MaxUsers: &maxUsers,
	}

	// Act
	resp, err := updateUseCase.Execute(ctx, tenant.ID, req)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 50, resp.MaxUsers)
}
