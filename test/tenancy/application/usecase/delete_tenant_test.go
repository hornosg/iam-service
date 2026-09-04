package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"iam/src/tenancy/application/usecase"
	"iam/src/tenancy/domain/entity"
	"iam/src/tenancy/domain/exception"
	tenantMother "iam/test/tenancy/domain/entity"
	"iam/test/tenancy/infrastructure/persistence/repository"
)

func TestDeleteTenantUseCase_Execute_HappyPath_DeletesTenant(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockTenantRepository()
	deleteUseCase := usecase.NewDeleteTenantUseCase(mockRepo)
	ctx := context.Background()

	mother := tenantMother.Create()
	tenant := mother.WithDefaults() // UserCount = 0
	mockRepo.SetupTenants([]*entity.Tenant{tenant})

	// Act
	err := deleteUseCase.Execute(ctx, tenant.ID)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
	assert.Equal(t, 1, mockRepo.GetCallCount("Update"))
}

func TestDeleteTenantUseCase_Execute_NotFound_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockTenantRepository()
	deleteUseCase := usecase.NewDeleteTenantUseCase(mockRepo)
	ctx := context.Background()

	// Act
	err := deleteUseCase.Execute(ctx, uuid.New())

	// Assert
	assert.Error(t, err)
	assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
}

func TestDeleteTenantUseCase_Execute_AlreadyDeleted_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockTenantRepository()
	deleteUseCase := usecase.NewDeleteTenantUseCase(mockRepo)
	ctx := context.Background()

	mother := tenantMother.Create()
	tenant := mother.Deleted()
	mockRepo.SetupTenants([]*entity.Tenant{tenant})

	// Act
	err := deleteUseCase.Execute(ctx, tenant.ID)

	// Assert
	assert.Error(t, err)
	assert.Equal(t, exception.ErrCannotDeleteTenant, err)
}

func TestDeleteTenantUseCase_Execute_WithActiveUsers_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockTenantRepository()
	deleteUseCase := usecase.NewDeleteTenantUseCase(mockRepo)
	ctx := context.Background()

	mother := tenantMother.Create()
	tenant := mother.WithUserLimits(10, 5) // 5 usuarios activos
	mockRepo.SetupTenants([]*entity.Tenant{tenant})

	// Act
	err := deleteUseCase.Execute(ctx, tenant.ID)

	// Assert
	assert.Error(t, err)
	assert.Equal(t, exception.ErrCannotDeleteTenant, err)
}

func TestDeleteTenantUseCase_Execute_UpdateFails_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockTenantRepository()
	deleteUseCase := usecase.NewDeleteTenantUseCase(mockRepo)
	ctx := context.Background()

	mother := tenantMother.Create()
	tenant := mother.WithDefaults()
	mockRepo.SetupTenants([]*entity.Tenant{tenant})
	mockRepo.ShouldFailOn("Update")

	// Act
	err := deleteUseCase.Execute(ctx, tenant.ID)

	// Assert
	assert.Error(t, err)
	assert.Equal(t, repository.ErrMockFailedOp, err)
}
