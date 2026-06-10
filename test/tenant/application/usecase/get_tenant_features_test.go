package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	srcEntity "iam/src/tenant/domain/entity"
	"iam/src/tenant/application/usecase"
	tenantMother "iam/test/tenant/domain/entity"
	"iam/test/tenant/infrastructure/persistence/repository"
)

func TestGetTenantFeaturesUseCase_Execute_ReturnsFeatures(t *testing.T) {
	mockRepo := repository.NewMockTenantRepository()
	uc := usecase.NewGetTenantFeaturesUseCase(mockRepo)
	ctx := context.Background()
	mother := tenantMother.Create()

	t.Run("debería retornar features de un tenant existente", func(t *testing.T) {
		tenant := mother.WithDefaults()
		mockRepo.SetupTenants([]*srcEntity.Tenant{tenant})
		mockRepo.ResetCallHistory()

		features, err := uc.Execute(ctx, tenant.ID)

		require.NoError(t, err)
		require.NotNil(t, features)
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
	})

	t.Run("debería retornar error si el tenant no existe", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.SetupTenants(nil)

		features, err := uc.Execute(ctx, uuid.New())

		assert.Error(t, err)
		assert.Nil(t, features)
	})

	t.Run("debería retornar error si el repositorio falla", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.ShouldFailOn("GetByID")

		features, err := uc.Execute(ctx, uuid.New())

		assert.Error(t, err)
		assert.Nil(t, features)
		assert.Equal(t, repository.ErrMockFailedOp, err)
	})
}
