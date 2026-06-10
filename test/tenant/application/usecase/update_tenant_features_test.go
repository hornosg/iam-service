package usecase_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	srcEntity "iam/src/tenant/domain/entity"
	"iam/src/tenant/application/usecase"
	tenantMother "iam/test/tenant/domain/entity"
	"iam/test/tenant/infrastructure/persistence/repository"
)

func TestUpdateTenantFeaturesUseCase_Execute_UpdatesFeatures(t *testing.T) {
	mockRepo := repository.NewMockTenantRepository()
	uc := usecase.NewUpdateTenantFeaturesUseCase(mockRepo)
	ctx := context.Background()
	mother := tenantMother.Create()

	t.Run("debería actualizar features del tenant", func(t *testing.T) {
		tenant := mother.WithDefaults()
		mockRepo.SetupTenants([]*srcEntity.Tenant{tenant})
		mockRepo.ResetCallHistory()

		req := &usecase.UpdateTenantFeaturesRequest{
			TenantID:         tenant.ID,
			FriendsFamily:    true,
			PremiumAnalytics: true,
		}

		resp, err := uc.Execute(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, tenant.ID, resp.ID)
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
		assert.Equal(t, 1, mockRepo.GetCallCount("Update"))
	})

	t.Run("debería desactivar features del tenant", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		tenant := mother.WithDefaults()
		mockRepo.SetupTenants([]*srcEntity.Tenant{tenant})

		req := &usecase.UpdateTenantFeaturesRequest{
			TenantID:         tenant.ID,
			FriendsFamily:    false,
			PremiumAnalytics: false,
		}

		resp, err := uc.Execute(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("debería retornar error si el tenant no existe", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.SetupTenants(nil)

		tenant := mother.WithDefaults()
		req := &usecase.UpdateTenantFeaturesRequest{
			TenantID: tenant.ID,
		}

		resp, err := uc.Execute(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
	})

	t.Run("debería retornar error si Update falla", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		tenant := mother.WithDefaults()
		mockRepo.SetupTenants([]*srcEntity.Tenant{tenant})
		mockRepo.ShouldFailOn("Update")

		req := &usecase.UpdateTenantFeaturesRequest{
			TenantID:         tenant.ID,
			PremiumAnalytics: true,
		}

		resp, err := uc.Execute(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, repository.ErrMockFailedOp, err)
	})
}
