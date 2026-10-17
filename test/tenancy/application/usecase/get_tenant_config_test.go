package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/hornosg/iam-service/src/tenancy/application/usecase"
	"github.com/hornosg/iam-service/src/tenancy/domain/entity"
	"github.com/hornosg/iam-service/test/tenancy/infrastructure/persistence/repository"
)

// Mudados de tenant-service (ACC-E04 T3, GetTenantConfigQuery en
// src/tenant/application/query/query_test.go). En iam la query es GetTenantConfigUseCase.

func TestGetTenantConfigUseCase_Execute_Found(t *testing.T) {
	// Arrange
	ctx := context.Background()
	tenantID := uuid.New()
	key := "catalog.stock_policy"
	mockRepo := new(repository.MockTenantConfigRepository)
	expectedConfig := entity.NewTenantConfig(tenantID, key, "IGNORE_STOCK")
	mockRepo.On("GetByKey", ctx, tenantID, key).Return(expectedConfig, true, nil)

	// Act
	config, exists, err := usecase.NewGetTenantConfigUseCase(mockRepo).Execute(ctx, tenantID, key)

	// Assert
	assert.NoError(t, err)
	assert.True(t, exists)
	assert.NotNil(t, config)
	assert.Equal(t, "IGNORE_STOCK", config.Value)
	mockRepo.AssertExpectations(t)
}

func TestGetTenantConfigUseCase_Execute_NotFound(t *testing.T) {
	// Arrange
	ctx := context.Background()
	tenantID := uuid.New()
	key := "nonexistent.key"
	mockRepo := new(repository.MockTenantConfigRepository)
	mockRepo.On("GetByKey", ctx, tenantID, key).Return(nil, false, nil)

	// Act
	config, exists, err := usecase.NewGetTenantConfigUseCase(mockRepo).Execute(ctx, tenantID, key)

	// Assert
	assert.NoError(t, err)
	assert.False(t, exists)
	assert.Nil(t, config)
}

func TestGetTenantConfigUseCase_Execute_Error(t *testing.T) {
	// Arrange
	ctx := context.Background()
	tenantID := uuid.New()
	key := "catalog.stock_policy"
	mockRepo := new(repository.MockTenantConfigRepository)
	mockRepo.On("GetByKey", ctx, tenantID, key).Return(nil, false, errors.New("db error"))

	// Act
	config, exists, err := usecase.NewGetTenantConfigUseCase(mockRepo).Execute(ctx, tenantID, key)

	// Assert
	assert.Error(t, err)
	assert.False(t, exists)
	assert.Nil(t, config)
}
