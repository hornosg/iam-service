package usecase_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	srcEntity "iam/src/tenant/domain/entity"
	"iam/src/tenant/application/usecase"
	"iam/src/tenant/domain/value_object"
	tenantMother "iam/test/tenant/domain/entity"
	"iam/test/tenant/infrastructure/persistence/repository"
)

func TestListTenantsUseCase_Execute_ReturnsPaginatedTenants(t *testing.T) {
	mockRepo := repository.NewMockTenantRepository()
	uc := usecase.NewListTenantsUseCase(mockRepo)
	ctx := context.Background()
	mother := tenantMother.Create()

	t.Run("debería listar tenants con paginación", func(t *testing.T) {
		tenants := []*srcEntity.Tenant{
			mother.WithSlug("tenant-a"),
			mother.WithSlug("tenant-b"),
		}
		mockRepo.SetupTenants(tenants)
		mockRepo.ResetCallHistory()

		resp, err := uc.Execute(ctx, 1, 10)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, 2, resp.TotalCount)
		assert.Equal(t, 1, mockRepo.GetCallCount("List"))
		assert.Equal(t, 1, mockRepo.GetCallCount("Count"))
	})

	t.Run("debería retornar error si List falla", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.ShouldFailOn("List")

		resp, err := uc.Execute(ctx, 1, 10)

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, repository.ErrMockFailedOp, err)
	})

	t.Run("debería retornar error si Count falla", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.SetupTenants([]*srcEntity.Tenant{mother.WithDefaults()})
		mockRepo.ShouldFailOn("Count")

		resp, err := uc.Execute(ctx, 1, 10)

		assert.Error(t, err)
		assert.Nil(t, resp)
	})
}

func TestListTenantsUseCase_GetByOwner_ReturnsTenants(t *testing.T) {
	mockRepo := repository.NewMockTenantRepository()
	uc := usecase.NewListTenantsUseCase(mockRepo)
	ctx := context.Background()
	mother := tenantMother.Create()
	tenant := mother.WithDefaults()

	t.Run("debería retornar tenants de un owner", func(t *testing.T) {
		mockRepo.SetupTenants([]*srcEntity.Tenant{tenant})
		mockRepo.ResetCallHistory()

		resp, err := uc.GetByOwner(ctx, tenant.OwnerID)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByOwner"))
	})

	t.Run("debería retornar error si GetByOwner falla", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.ShouldFailOn("GetByOwner")

		resp, err := uc.GetByOwner(ctx, tenant.OwnerID)

		assert.Error(t, err)
		assert.Nil(t, resp)
	})
}

func TestListTenantsUseCase_GetByStatus_ReturnsTenants(t *testing.T) {
	mockRepo := repository.NewMockTenantRepository()
	uc := usecase.NewListTenantsUseCase(mockRepo)
	ctx := context.Background()
	mother := tenantMother.Create()

	t.Run("debería retornar tenants por status", func(t *testing.T) {
		mockRepo.SetupTenants([]*srcEntity.Tenant{mother.WithDefaults()})
		mockRepo.ResetCallHistory()

		resp, err := uc.GetByStatus(ctx, value_object.TenantStatusActive, 1, 10)

		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("debería retornar error si GetByStatus falla", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.ShouldFailOn("GetByStatus")

		resp, err := uc.GetByStatus(ctx, value_object.TenantStatusActive, 1, 10)

		assert.Error(t, err)
		assert.Nil(t, resp)
	})
}

func TestListTenantsUseCase_GetByType_ReturnsTenants(t *testing.T) {
	mockRepo := repository.NewMockTenantRepository()
	uc := usecase.NewListTenantsUseCase(mockRepo)
	ctx := context.Background()
	mother := tenantMother.Create()

	t.Run("debería retornar tenants por tipo", func(t *testing.T) {
		mockRepo.SetupTenants([]*srcEntity.Tenant{mother.WithDefaults()})
		mockRepo.ResetCallHistory()

		resp, err := uc.GetByType(ctx, value_object.TenantTypePersonal, 1, 10)

		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("debería retornar error si GetByType falla", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.ShouldFailOn("GetByType")

		resp, err := uc.GetByType(ctx, value_object.TenantTypePersonal, 1, 10)

		assert.Error(t, err)
		assert.Nil(t, resp)
	})
}

func TestListTenantsUseCase_GetActive_ReturnsTenants(t *testing.T) {
	mockRepo := repository.NewMockTenantRepository()
	uc := usecase.NewListTenantsUseCase(mockRepo)
	ctx := context.Background()
	mother := tenantMother.Create()

	t.Run("debería retornar tenants activos", func(t *testing.T) {
		mockRepo.SetupTenants([]*srcEntity.Tenant{mother.WithDefaults()})
		mockRepo.ResetCallHistory()

		resp, err := uc.GetActive(ctx, 1, 10)

		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("debería retornar error si GetActive falla", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.ShouldFailOn("GetByStatus")

		resp, err := uc.GetActive(ctx, 1, 10)

		assert.Error(t, err)
		assert.Nil(t, resp)
	})
}

func TestListTenantsUseCase_GetExpiring_ReturnsTenants(t *testing.T) {
	mockRepo := repository.NewMockTenantRepository()
	uc := usecase.NewListTenantsUseCase(mockRepo)
	ctx := context.Background()

	t.Run("debería retornar tenants próximos a expirar", func(t *testing.T) {
		mockRepo.SetupTenants(nil)
		mockRepo.ResetCallHistory()

		resp, err := uc.GetExpiring(ctx, 30)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, 1, mockRepo.GetCallCount("GetExpiring"))
	})

	t.Run("debería retornar error si GetExpiring falla", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.ShouldFailOn("GetExpiring")

		resp, err := uc.GetExpiring(ctx, 30)

		assert.Error(t, err)
		assert.Nil(t, resp)
	})
}
