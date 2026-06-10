package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	srcEntity "iam/src/tenant/domain/entity"
	"iam/src/tenant/application/request"
	"iam/src/tenant/application/usecase"
	"iam/src/tenant/domain/exception"
	"iam/src/tenant/domain/value_object"
	tenantMother "iam/test/tenant/domain/entity"
	"iam/test/tenant/infrastructure/persistence/repository"
)

func TestSetPlanUseCase_Execute_AssignsPlanToTenant(t *testing.T) {
	mockRepo := repository.NewMockTenantRepository()
	uc := usecase.NewSetPlanUseCase(mockRepo)
	ctx := context.Background()
	mother := tenantMother.Create()

	t.Run("debería asignar plan a tenant activo", func(t *testing.T) {
		tenant := mother.WithDefaults()
		mockRepo.SetupTenants([]*srcEntity.Tenant{tenant})
		mockRepo.ResetCallHistory()

		planID := uuid.New()
		req := &request.SetPlanRequest{PlanID: planID.String()}

		resp, err := uc.Execute(ctx, tenant.ID, req)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, tenant.ID, resp.ID)
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
		assert.Equal(t, 1, mockRepo.GetCallCount("Update"))
	})

	t.Run("debería retornar error si el tenant no existe", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.SetupTenants(nil)

		req := &request.SetPlanRequest{PlanID: uuid.New().String()}

		resp, err := uc.Execute(ctx, uuid.New(), req)

		assert.Error(t, err)
		assert.Nil(t, resp)
	})

	t.Run("debería retornar error si el tenant está eliminado", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		tenant := mother.WithStatus(value_object.TenantStatusDeleted)
		mockRepo.SetupTenants([]*srcEntity.Tenant{tenant})

		req := &request.SetPlanRequest{PlanID: uuid.New().String()}

		resp, err := uc.Execute(ctx, tenant.ID, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, exception.ErrTenantDeleted, err)
	})

	t.Run("debería retornar error si el tenant no está activo", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		tenant := mother.WithStatus(value_object.TenantStatusInactive)
		mockRepo.SetupTenants([]*srcEntity.Tenant{tenant})

		req := &request.SetPlanRequest{PlanID: uuid.New().String()}

		resp, err := uc.Execute(ctx, tenant.ID, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, exception.ErrTenantNotActive, err)
	})

	t.Run("debería retornar error con plan_id inválido", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		tenant := mother.WithDefaults()
		mockRepo.SetupTenants([]*srcEntity.Tenant{tenant})

		req := &request.SetPlanRequest{PlanID: "not-a-uuid"}

		resp, err := uc.Execute(ctx, tenant.ID, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, exception.ErrPlanNotFound, err)
	})

	t.Run("debería retornar error si Update falla", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		tenant := mother.WithDefaults()
		mockRepo.SetupTenants([]*srcEntity.Tenant{tenant})
		mockRepo.ShouldFailOn("Update")

		req := &request.SetPlanRequest{PlanID: uuid.New().String()}

		resp, err := uc.Execute(ctx, tenant.ID, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
	})
}

func TestSetPlanUseCase_RemovePlan_RemovesPlanFromTenant(t *testing.T) {
	mockRepo := repository.NewMockTenantRepository()
	uc := usecase.NewSetPlanUseCase(mockRepo)
	ctx := context.Background()
	mother := tenantMother.Create()

	t.Run("debería remover plan de un tenant", func(t *testing.T) {
		tenant := mother.WithDefaults()
		mockRepo.SetupTenants([]*srcEntity.Tenant{tenant})
		mockRepo.ResetCallHistory()

		resp, err := uc.RemovePlan(ctx, tenant.ID)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
		assert.Equal(t, 1, mockRepo.GetCallCount("Update"))
	})

	t.Run("debería retornar error si el tenant no existe", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.SetupTenants(nil)

		resp, err := uc.RemovePlan(ctx, uuid.New())

		assert.Error(t, err)
		assert.Nil(t, resp)
	})

	t.Run("debería retornar error si el tenant está eliminado", func(t *testing.T) {
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		tenant := mother.WithStatus(value_object.TenantStatusDeleted)
		mockRepo.SetupTenants([]*srcEntity.Tenant{tenant})

		resp, err := uc.RemovePlan(ctx, tenant.ID)

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, exception.ErrTenantDeleted, err)
	})
}
