package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/hornosg/iam-service/src/tenancy/application/usecase"
	"github.com/hornosg/iam-service/src/tenancy/domain/entity"
	"github.com/hornosg/iam-service/test/tenancy/infrastructure/persistence/repository"
)

// Mudados de tenant-service (ACC-E04 T3). El bootstrap ya no hace GetByKey + Save: delega en
// InsertMissing (ADR-005 §Bootstrap atómico), así que las expectativas del mock cambian; los
// casos (tenant nuevo, existente, doble llamada, error de lectura, error de escritura) se conservan.

// defaultsFor verifica que el lote sea exactamente DefaultConfigs para el tenant pedido.
func defaultsFor(tenantID uuid.UUID) interface{} {
	return mock.MatchedBy(func(configs []*entity.TenantConfig) bool {
		if len(configs) != len(usecase.DefaultConfigs) {
			return false
		}
		for _, c := range configs {
			if c.TenantID != tenantID || usecase.DefaultConfigs[c.Key] != c.Value {
				return false
			}
		}
		return true
	})
}

// TestBootstrapTenantConfig_NewTenant prueba el bootstrap para un tenant nuevo
func TestBootstrapTenantConfig_NewTenant(t *testing.T) {
	ctx := context.Background()
	tenantID := uuid.New()
	mockRepo := new(repository.MockTenantConfigRepository)
	mockRepo.On("InsertMissing", ctx, tenantID, defaultsFor(tenantID)).Return(1, nil)

	createdCount, err := usecase.NewBootstrapTenantConfigUseCase(mockRepo).Execute(ctx, tenantID)

	assert.NoError(t, err)
	assert.Equal(t, 1, createdCount, "Should create 1 config for new tenant")
	mockRepo.AssertExpectations(t)
}

// TestBootstrapTenantConfig_ExistingTenant prueba la idempotencia: sin pre-read ni upsert
func TestBootstrapTenantConfig_ExistingTenant(t *testing.T) {
	ctx := context.Background()
	tenantID := uuid.New()
	mockRepo := new(repository.MockTenantConfigRepository)
	mockRepo.On("InsertMissing", ctx, tenantID, defaultsFor(tenantID)).Return(0, nil)

	createdCount, err := usecase.NewBootstrapTenantConfigUseCase(mockRepo).Execute(ctx, tenantID)

	assert.NoError(t, err)
	assert.Equal(t, 0, createdCount, "Should not create any config for existing tenant")
	mockRepo.AssertExpectations(t)
	mockRepo.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
	mockRepo.AssertNotCalled(t, "GetByKey", mock.Anything, mock.Anything, mock.Anything)
}

// TestBootstrapTenantConfig_DoubleCall prueba llamar bootstrap dos veces
func TestBootstrapTenantConfig_DoubleCall(t *testing.T) {
	ctx := context.Background()
	tenantID := uuid.New()
	mockRepo := new(repository.MockTenantConfigRepository)
	mockRepo.On("InsertMissing", ctx, tenantID, defaultsFor(tenantID)).Return(1, nil).Once()
	mockRepo.On("InsertMissing", ctx, tenantID, defaultsFor(tenantID)).Return(0, nil).Once()
	uc := usecase.NewBootstrapTenantConfigUseCase(mockRepo)

	createdCount1, err1 := uc.Execute(ctx, tenantID)
	assert.NoError(t, err1)
	assert.Equal(t, 1, createdCount1, "First call should create 1 config")

	createdCount2, err2 := uc.Execute(ctx, tenantID)
	assert.NoError(t, err2)
	assert.Equal(t, 0, createdCount2, "Second call should not create any config (idempotent)")

	mockRepo.AssertExpectations(t)
}

// TestBootstrapTenantConfig_RepositoryError prueba manejo de errores
func TestBootstrapTenantConfig_RepositoryError(t *testing.T) {
	ctx := context.Background()
	tenantID := uuid.New()
	mockRepo := new(repository.MockTenantConfigRepository)
	mockRepo.On("InsertMissing", ctx, tenantID, defaultsFor(tenantID)).Return(0, errors.New("database error"))

	createdCount, err := usecase.NewBootstrapTenantConfigUseCase(mockRepo).Execute(ctx, tenantID)

	assert.Error(t, err)
	assert.Equal(t, 0, createdCount)
	assert.Contains(t, err.Error(), "database error")
	mockRepo.AssertExpectations(t)
}

// TestBootstrapTenantConfig_SaveError prueba error al guardar: ante error el contador es 0
// aunque el repositorio reporte filas (el lote se revirtió).
func TestBootstrapTenantConfig_SaveError(t *testing.T) {
	ctx := context.Background()
	tenantID := uuid.New()
	mockRepo := new(repository.MockTenantConfigRepository)
	mockRepo.On("InsertMissing", ctx, tenantID, defaultsFor(tenantID)).Return(1, errors.New("save error"))

	createdCount, err := usecase.NewBootstrapTenantConfigUseCase(mockRepo).Execute(ctx, tenantID)

	assert.Error(t, err)
	assert.Equal(t, 0, createdCount)
	assert.Contains(t, err.Error(), "save error")
	mockRepo.AssertExpectations(t)
}
