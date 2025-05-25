package usecase_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"iam/src/plan/application/request"
	"iam/src/plan/application/usecase"
	"iam/src/plan/domain/entity"
	"iam/src/plan/domain/exception"
	planEntity "iam/test/plan/domain/entity"
	"iam/test/plan/infrastructure/persistence/repository"
)

func TestCreatePlanUseCase_Execute(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockPlanRepository()
	createUseCase := usecase.NewCreatePlanUseCase(mockRepo)
	ctx := context.Background()
	planMother := planEntity.Create()

	t.Run("debería crear un plan básico con éxito", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		req := &request.CreatePlanRequest{
			Name:        "Plan Básico Test",
			Description: "Plan básico para testing",
			Type:        "BASIC",
			PriceMonth:  9.99,
			PriceYear:   99.99,
			Features:    []string{"feature1", "feature2"},
		}

		// Act
		planResponse, err := createUseCase.Execute(ctx, req)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, planResponse)
		assert.Equal(t, req.Name, planResponse.Name)
		assert.Equal(t, req.Description, planResponse.Description)
		assert.Equal(t, req.Type, planResponse.Type)
		assert.Equal(t, req.PriceMonth, planResponse.PriceMonth)
		assert.Equal(t, req.PriceYear, planResponse.PriceYear)
		assert.Equal(t, "ACTIVE", planResponse.Status)
		assert.Equal(t, 10, planResponse.MaxUsers) // BASIC permite 10 usuarios
		assert.Equal(t, req.Features, planResponse.Features)
		assert.NotEmpty(t, planResponse.ID)

		// Verificar llamadas al repositorio
		assert.Equal(t, 1, mockRepo.GetCallCount("ExistsByName"))
		assert.Equal(t, 1, mockRepo.GetCallCount("Create"))

		// Verificar que el plan está en el repositorio
		plans := mockRepo.GetPlans()
		assert.Len(t, plans, 1)
		assert.Equal(t, planResponse.ID, plans[0].ID)
	})

	t.Run("debería crear un plan gratuito con éxito", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		req := &request.CreatePlanRequest{
			Name:        "Plan Gratuito",
			Description: "Plan gratuito para usuarios individuales",
			Type:        "FREE",
			PriceMonth:  0.0,
			PriceYear:   0.0,
			Features:    []string{"basic_feature"},
		}

		// Act
		planResponse, err := createUseCase.Execute(ctx, req)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, planResponse)
		assert.Equal(t, "FREE", planResponse.Type)
		assert.Equal(t, 1, planResponse.MaxUsers) // FREE permite 1 usuario
		assert.Equal(t, 0.0, planResponse.PriceMonth)
		assert.Equal(t, 0.0, planResponse.PriceYear)
	})

	t.Run("debería crear un plan enterprise con éxito", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		req := &request.CreatePlanRequest{
			Name:        "Plan Enterprise",
			Description: "Plan enterprise para grandes organizaciones",
			Type:        "ENTERPRISE",
			PriceMonth:  99.99,
			PriceYear:   999.99,
			Features:    []string{"enterprise_feature1", "enterprise_feature2", "sso"},
		}

		// Act
		planResponse, err := createUseCase.Execute(ctx, req)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, planResponse)
		assert.Equal(t, "ENTERPRISE", planResponse.Type)
		assert.Equal(t, -1, planResponse.MaxUsers) // ENTERPRISE permite usuarios ilimitados
		assert.Len(t, planResponse.Features, 3)
	})

	t.Run("debería fallar si el plan ya existe", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		existingPlan := planMother.WithName("Plan Existente")
		mockRepo.SetupPlans([]*entity.Plan{existingPlan})

		req := &request.CreatePlanRequest{
			Name:        "Plan Existente",
			Description: "Intentando crear plan duplicado",
			Type:        "BASIC",
			PriceMonth:  9.99,
			PriceYear:   99.99,
		}

		// Act
		planResponse, err := createUseCase.Execute(ctx, req)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, planResponse)
		assert.Equal(t, exception.ErrPlanAlreadyExists, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("ExistsByName"))
		assert.Equal(t, 0, mockRepo.GetCallCount("Create")) // No debe llamar a Create
	})

	t.Run("debería fallar con tipo de plan inválido", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		req := &request.CreatePlanRequest{
			Name:        "Plan Inválido",
			Description: "Plan con tipo inválido",
			Type:        "INVALID_TYPE",
			PriceMonth:  9.99,
			PriceYear:   99.99,
		}

		// Act
		planResponse, err := createUseCase.Execute(ctx, req)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, planResponse)
		assert.Equal(t, exception.ErrInvalidPlanType, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("ExistsByName"))
		assert.Equal(t, 0, mockRepo.GetCallCount("Create"))
	})

	t.Run("debería fallar si ExistsByName falla", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.ShouldFailOn("ExistsByName")

		req := &request.CreatePlanRequest{
			Name:        "Plan Test",
			Description: "Plan de prueba",
			Type:        "BASIC",
			PriceMonth:  9.99,
			PriceYear:   99.99,
		}

		// Act
		planResponse, err := createUseCase.Execute(ctx, req)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, planResponse)
		assert.Equal(t, repository.ErrMockFailedOp, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("ExistsByName"))
		assert.Equal(t, 0, mockRepo.GetCallCount("Create"))
	})

	t.Run("debería fallar si Create falla", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.ShouldFailOn("Create")

		req := &request.CreatePlanRequest{
			Name:        "Plan Test",
			Description: "Plan de prueba",
			Type:        "BASIC",
			PriceMonth:  9.99,
			PriceYear:   99.99,
		}

		// Act
		planResponse, err := createUseCase.Execute(ctx, req)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, planResponse)
		assert.Equal(t, repository.ErrMockFailedOp, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("ExistsByName"))
		assert.Equal(t, 1, mockRepo.GetCallCount("Create"))
	})

	t.Run("debería crear plan sin features", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		req := &request.CreatePlanRequest{
			Name:        "Plan Sin Features",
			Description: "Plan básico sin features adicionales",
			Type:        "BASIC",
			PriceMonth:  9.99,
			PriceYear:   99.99,
			Features:    nil, // Sin features
		}

		// Act
		planResponse, err := createUseCase.Execute(ctx, req)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, planResponse)
		assert.Empty(t, planResponse.Features)
		assert.Equal(t, 1, mockRepo.GetCallCount("ExistsByName"))
		assert.Equal(t, 1, mockRepo.GetCallCount("Create"))
	})

	t.Run("debería calcular descuento anual correctamente", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		req := &request.CreatePlanRequest{
			Name:        "Plan Con Descuento",
			Description: "Plan con descuento anual",
			Type:        "PREMIUM",
			PriceMonth:  29.99,
			PriceYear:   299.99, // 10 meses de precio (16.67% descuento)
		}

		// Act
		planResponse, err := createUseCase.Execute(ctx, req)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, planResponse)
		assert.Greater(t, planResponse.YearlyDiscount, 15.0) // Debería tener descuento
		assert.Less(t, planResponse.YearlyDiscount, 20.0)
	})
}
