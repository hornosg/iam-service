package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"iam/src/identity/application/usecase"
	"iam/src/identity/domain/entity"
	"iam/src/identity/domain/value_object"
	userMother "iam/test/identity/user/domain/entity"
	"iam/test/identity/user/infrastructure/persistence/repository"
)

func TestListUsersUseCase_Execute_ByTenant_ReturnsUsers(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	listUseCase := usecase.NewListUsersUseCase(mockRepo)
	ctx := context.Background()

	mother := userMother.Create()
	tenantID := uuid.New()
	user1 := mother.WithTenant(tenantID)
	user2 := mother.WithTenant(tenantID)
	mockRepo.SetupUsers([]*entity.User{user1, user2})

	params := &usecase.ListUsersParams{
		TenantID: &tenantID,
		Page:     1,
		PageSize: 10,
	}

	// Act
	resp, err := listUseCase.Execute(ctx, params)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.GreaterOrEqual(t, len(resp.Users), 1)
	assert.Equal(t, 1, mockRepo.GetCallCount("GetByTenant"))
	assert.Equal(t, 1, mockRepo.GetCallCount("CountByTenant"))
}

func TestListUsersUseCase_Execute_ByStatus_ReturnsUsers(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	listUseCase := usecase.NewListUsersUseCase(mockRepo)
	ctx := context.Background()

	mother := userMother.Create()
	user := mother.WithStatus(value_object.StatusActive)
	mockRepo.SetupUsers([]*entity.User{user})

	status := value_object.StatusActive
	params := &usecase.ListUsersParams{
		Status:   &status,
		Page:     1,
		PageSize: 10,
	}

	// Act
	resp, err := listUseCase.Execute(ctx, params)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 1, mockRepo.GetCallCount("GetByStatus"))
}

func TestListUsersUseCase_Execute_ByRole_ReturnsUsers(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	listUseCase := usecase.NewListUsersUseCase(mockRepo)
	ctx := context.Background()

	mother := userMother.Create()
	roleID := uuid.New()
	user := mother.WithRole(roleID)
	mockRepo.SetupUsers([]*entity.User{user})

	params := &usecase.ListUsersParams{
		RoleID:   &roleID,
		Page:     1,
		PageSize: 10,
	}

	// Act
	resp, err := listUseCase.Execute(ctx, params)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 1, mockRepo.GetCallCount("GetByRole"))
}

func TestListUsersUseCase_Execute_NoFilter_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	listUseCase := usecase.NewListUsersUseCase(mockRepo)
	ctx := context.Background()

	params := &usecase.ListUsersParams{
		Page:     1,
		PageSize: 10,
	}

	// Act
	resp, err := listUseCase.Execute(ctx, params)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "filtro")
}

func TestListUsersUseCase_Execute_EmptyResult_ReturnsEmptyList(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	listUseCase := usecase.NewListUsersUseCase(mockRepo)
	ctx := context.Background()

	tenantID := uuid.New()
	params := &usecase.ListUsersParams{
		TenantID: &tenantID,
		Page:     1,
		PageSize: 10,
	}

	// Act
	resp, err := listUseCase.Execute(ctx, params)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Users)
	assert.Equal(t, 0, resp.Total)
}

func TestListUsersParams_GetOffset_CalculatesCorrectly(t *testing.T) {
	tests := []struct {
		page     int
		pageSize int
		expected int
	}{
		{1, 10, 0},
		{2, 10, 10},
		{3, 20, 40},
		{0, 10, 0},  // Page <= 1
		{-1, 10, 0}, // Page <= 1
	}

	for _, tt := range tests {
		params := &usecase.ListUsersParams{Page: tt.page, PageSize: tt.pageSize}
		assert.Equal(t, tt.expected, params.GetOffset())
	}
}

func TestListUsersParams_GetLimit_RespectsLimits(t *testing.T) {
	tests := []struct {
		pageSize int
		expected int
	}{
		{10, 10},
		{0, 10},    // Default
		{-1, 10},   // Default
		{100, 100}, // Max
		{200, 100}, // Capped at max
	}

	for _, tt := range tests {
		params := &usecase.ListUsersParams{PageSize: tt.pageSize}
		assert.Equal(t, tt.expected, params.GetLimit())
	}
}
