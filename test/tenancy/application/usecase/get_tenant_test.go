package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"iam/src/tenancy/application/usecase"
	"iam/src/tenancy/domain/entity"
	tenantMother "iam/test/tenancy/domain/entity"
	"iam/test/tenancy/infrastructure/persistence/repository"
)

func TestGetTenantByIDUseCase_Execute_Found_ReturnsResponse(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockTenantRepository()
	getUseCase := usecase.NewGetTenantByIDUseCase(mockRepo)
	ctx := context.Background()

	mother := tenantMother.Create()
	tenant := mother.WithName("Tenant Encontrado")
	mockRepo.SetupTenants([]*entity.Tenant{tenant})

	// Act
	resp, err := getUseCase.Execute(ctx, tenant.ID)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, tenant.ID, resp.ID)
	assert.Equal(t, "Tenant Encontrado", resp.Name)
	assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
}

func TestGetTenantByIDUseCase_Execute_NotFound_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockTenantRepository()
	getUseCase := usecase.NewGetTenantByIDUseCase(mockRepo)
	ctx := context.Background()

	// Act
	resp, err := getUseCase.Execute(ctx, uuid.New())

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
}

func TestGetTenantBySlugUseCase_Execute_Found_ReturnsResponse(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockTenantRepository()
	getBySlugUseCase := usecase.NewGetTenantBySlugUseCase(mockRepo)
	ctx := context.Background()

	mother := tenantMother.Create()
	tenant := mother.WithSlug("mi-tienda")
	mockRepo.SetupTenants([]*entity.Tenant{tenant})

	// Act
	resp, err := getBySlugUseCase.Execute(ctx, "mi-tienda")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "mi-tienda", resp.Slug)
	assert.Equal(t, 1, mockRepo.GetCallCount("GetBySlug"))
}

func TestGetTenantBySlugUseCase_Execute_NotFound_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockTenantRepository()
	getBySlugUseCase := usecase.NewGetTenantBySlugUseCase(mockRepo)
	ctx := context.Background()

	// Act
	resp, err := getBySlugUseCase.Execute(ctx, "nonexistent-slug")

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestGetTenantBySlugUseCase_Execute_RepoFails_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockTenantRepository()
	getBySlugUseCase := usecase.NewGetTenantBySlugUseCase(mockRepo)
	ctx := context.Background()

	mockRepo.ShouldFailOn("GetBySlug")

	// Act
	resp, err := getBySlugUseCase.Execute(ctx, "any-slug")

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, repository.ErrMockFailedOp, err)
}
