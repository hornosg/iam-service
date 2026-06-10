package usecase_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	srcEntity "iam/src/plan/domain/entity"
	"iam/src/plan/application/usecase"
	planEntity "iam/test/plan/domain/entity"
	"iam/test/plan/infrastructure/persistence/repository"
)

func TestListPlansUseCase_Execute_ReturnsPaginatedPlans(t *testing.T) {
	mockRepo := repository.NewMockPlanRepository()
	uc := usecase.NewListPlansUseCase(mockRepo)
	ctx := context.Background()
	mother := planEntity.Create()

	t.Run("debería listar planes con paginación", func(t *testing.T) {
		plans := []*srcEntity.Plan{
			mother.WithName("Plan A"),
			mother.WithName("Plan B"),
		}
		mockRepo.SetupPlans(plans)
		mockRepo.ResetCallHistory()

		resp, err := uc.Execute(ctx, 1, 10)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, 2, resp.TotalCount)
		assert.Equal(t, 1, mockRepo.GetCallCount("List"))
		assert.Equal(t, 1, mockRepo.GetCallCount("Count"))
	})

	t.Run("debería retornar lista vacía cuando no hay planes", func(t *testing.T) {
		mockRepo.SetupPlans(nil)
		mockRepo.ResetCallHistory()

		resp, err := uc.Execute(ctx, 1, 10)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, 0, resp.TotalCount)
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
		mockRepo.SetupPlans([]*srcEntity.Plan{mother.WithDefaults()})
		mockRepo.ShouldFailOn("Count")

		resp, err := uc.Execute(ctx, 1, 10)

		assert.Error(t, err)
		assert.Nil(t, resp)
	})
}

func TestListPlansUseCase_GetActive_ReturnsActivePlans(t *testing.T) {
	mockRepo := repository.NewMockPlanRepository()
	uc := usecase.NewListPlansUseCase(mockRepo)
	ctx := context.Background()
	mother := planEntity.Create()

	t.Run("debería retornar los planes activos", func(t *testing.T) {
		mockRepo.SetupPlans([]*srcEntity.Plan{mother.WithDefaults()})
		mockRepo.ResetCallHistory()

		resp, err := uc.GetActive(ctx)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.GreaterOrEqual(t, resp.TotalCount, 0)
	})

	t.Run("debería retornar error si GetByStatus falla", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.ShouldFailOn("GetByStatus")

		resp, err := uc.GetActive(ctx)

		assert.Error(t, err)
		assert.Nil(t, resp)
	})
}
