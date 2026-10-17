package usecase_test

import (
	"context"
	"errors"
	"github.com/hornosg/iam-service/src/tenancy/application/usecase"
	"github.com/hornosg/iam-service/src/tenancy/domain/entity"
	"github.com/hornosg/iam-service/test/tenancy/infrastructure/persistence/repository"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// TestSetTenantConfigCommand_Execute_Insert tests inserting a new config
func TestSetTenantConfigCommand_Execute_Insert(t *testing.T) {
	// Arrange
	mockRepo := new(repository.MockTenantConfigRepository)
	command := usecase.NewSetTenantConfigUseCase(mockRepo)

	tenantID := uuid.New()
	key := "catalog.stock_policy"
	value := "IGNORE_STOCK"
	ctx := context.Background()

	// Config no existe
	mockRepo.On("GetByKey", ctx, tenantID, key).Return(nil, false, nil)
	mockRepo.On("Save", ctx, mock.AnythingOfType("*entity.TenantConfig")).Return(nil)

	// Act
	result, err := command.Execute(ctx, tenantID, key, value)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, tenantID, result.TenantID)
	assert.Equal(t, key, result.Key)
	assert.Equal(t, value, result.Value)
	mockRepo.AssertExpectations(t)
}

// TestSetTenantConfigCommand_Execute_Update tests updating an existing config
func TestSetTenantConfigCommand_Execute_Update(t *testing.T) {
	// Arrange
	mockRepo := new(repository.MockTenantConfigRepository)
	command := usecase.NewSetTenantConfigUseCase(mockRepo)

	tenantID := uuid.New()
	key := "catalog.stock_policy"
	oldValue := "ENFORCE_STOCK"
	newValue := "IGNORE_STOCK"
	ctx := context.Background()

	existingConfig := entity.NewTenantConfig(tenantID, key, oldValue)

	// Config existe
	mockRepo.On("GetByKey", ctx, tenantID, key).Return(existingConfig, true, nil)
	mockRepo.On("Save", ctx, mock.AnythingOfType("*entity.TenantConfig")).Return(nil)

	// Act
	result, err := command.Execute(ctx, tenantID, key, newValue)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, tenantID, result.TenantID)
	assert.Equal(t, key, result.Key)
	assert.Equal(t, newValue, result.Value)
	assert.Equal(t, existingConfig.ID, result.ID) // Mismo ID
	mockRepo.AssertExpectations(t)
}

// TestSetTenantConfigCommand_Execute_GetByKeyError tests error handling when GetByKey fails
func TestSetTenantConfigCommand_Execute_GetByKeyError(t *testing.T) {
	// Arrange
	mockRepo := new(repository.MockTenantConfigRepository)
	command := usecase.NewSetTenantConfigUseCase(mockRepo)

	tenantID := uuid.New()
	key := "catalog.stock_policy"
	value := "IGNORE_STOCK"
	ctx := context.Background()

	expectedError := errors.New("database connection error")
	mockRepo.On("GetByKey", ctx, tenantID, key).Return(nil, false, expectedError)

	// Act
	result, err := command.Execute(ctx, tenantID, key, value)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, expectedError, err)
	mockRepo.AssertExpectations(t)
}

// TestSetTenantConfigCommand_Execute_SaveError tests error handling when Save fails
func TestSetTenantConfigCommand_Execute_SaveError(t *testing.T) {
	// Arrange
	mockRepo := new(repository.MockTenantConfigRepository)
	command := usecase.NewSetTenantConfigUseCase(mockRepo)

	tenantID := uuid.New()
	key := "catalog.stock_policy"
	value := "IGNORE_STOCK"
	ctx := context.Background()

	expectedError := errors.New("save failed")
	mockRepo.On("GetByKey", ctx, tenantID, key).Return(nil, false, nil)
	mockRepo.On("Save", ctx, mock.AnythingOfType("*entity.TenantConfig")).Return(expectedError)

	// Act
	result, err := command.Execute(ctx, tenantID, key, value)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, expectedError, err)
	mockRepo.AssertExpectations(t)
}
