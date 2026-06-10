package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"iam/src/plan/application/usecase"
	planEntity "iam/test/plan/domain/entity"
	"iam/test/plan/infrastructure/persistence/repository"
	srcEntity "iam/src/plan/domain/entity"
)

func TestGetPlanByIDUseCase_Execute_ExistingPlan_ReturnsPlan(t *testing.T) {
	mockRepo := repository.NewMockPlanRepository()
	uc := usecase.NewGetPlanByIDUseCase(mockRepo)
	ctx := context.Background()
	mother := planEntity.Create()

	t.Run("debería retornar el plan cuando existe", func(t *testing.T) {
		plan := mother.WithDefaults()
		mockRepo.SetupPlans([]*srcEntity.Plan{plan})
		mockRepo.ResetCallHistory()

		resp, err := uc.Execute(ctx, plan.ID)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, plan.ID, resp.ID)
		assert.Equal(t, plan.Name, resp.Name)
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
	})

	t.Run("debería retornar error cuando el plan no existe", func(t *testing.T) {
		mockRepo.SetupPlans(nil)
		mockRepo.ResetCallHistory()

		resp, err := uc.Execute(ctx, uuid.New())

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
	})

	t.Run("debería retornar error si el repositorio falla", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.ShouldFailOn("GetByID")

		resp, err := uc.Execute(ctx, uuid.New())

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, repository.ErrMockFailedOp, err)
	})
}
