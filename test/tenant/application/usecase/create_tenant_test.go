package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"iam/src/tenant/application/request"
	"iam/src/tenant/application/usecase"
	"iam/src/tenant/domain/entity"
	"iam/src/tenant/domain/exception"
	sharedmetrics "github.com/mercadocercano/go-shared/infrastructure/metrics"
	tenantMother "iam/test/tenant/domain/entity"
	"iam/test/tenant/infrastructure/persistence/repository"
)

func TestCreateTenantUseCase_Execute_HappyPath_CreatesTenant(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockTenantRepository()
	createUseCase := usecase.NewCreateTenantUseCase(mockRepo, sharedmetrics.NoopRecorder{})
	ctx := context.Background()

	ownerID := uuid.New()
	req := &request.CreateTenantRequest{
		Name:        "Mi Tienda",
		Slug:        "mi-tienda",
		Description: "Una tienda de prueba",
		Type:        "STARTUP",
		OwnerID:     ownerID.String(),
	}

	// Act
	resp, err := createUseCase.Execute(ctx, req)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "Mi Tienda", resp.Name)
	assert.Equal(t, "mi-tienda", resp.Slug)
	assert.Equal(t, "STARTUP", resp.Type)
	assert.Equal(t, "ACTIVE", resp.Status)
	assert.Equal(t, 10, resp.MaxUsers) // Startup default
	assert.True(t, resp.IsActive)
	assert.Equal(t, 1, mockRepo.GetCallCount("ExistsBySlug"))
	assert.Equal(t, 1, mockRepo.GetCallCount("Create"))
}

func TestCreateTenantUseCase_Execute_SlugDuplicate_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockTenantRepository()
	createUseCase := usecase.NewCreateTenantUseCase(mockRepo, sharedmetrics.NoopRecorder{})
	ctx := context.Background()

	mother := tenantMother.Create()
	existing := mother.WithSlug("existing-slug")
	mockRepo.SetupTenants([]*entity.Tenant{existing})

	ownerID := uuid.New()
	req := &request.CreateTenantRequest{
		Name:        "Otro Tenant",
		Slug:        "existing-slug",
		Description: "Intento de duplicado",
		Type:        "PERSONAL",
		OwnerID:     ownerID.String(),
	}

	// Act
	resp, err := createUseCase.Execute(ctx, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, exception.ErrSlugAlreadyExists, err)
}

func TestCreateTenantUseCase_Execute_DomainDuplicate_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockTenantRepository()
	createUseCase := usecase.NewCreateTenantUseCase(mockRepo, sharedmetrics.NoopRecorder{})
	ctx := context.Background()

	mother := tenantMother.Create()
	existing := mother.WithDomain("existing.com")
	mockRepo.SetupTenants([]*entity.Tenant{existing})

	ownerID := uuid.New()
	req := &request.CreateTenantRequest{
		Name:        "Nuevo Tenant",
		Slug:        "nuevo-tenant",
		Description: "Descripcion del tenant",
		Type:        "BUSINESS",
		Domain:      "existing.com",
		OwnerID:     ownerID.String(),
	}

	// Act
	resp, err := createUseCase.Execute(ctx, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, exception.ErrDomainAlreadyExists, err)
}

func TestCreateTenantUseCase_Execute_InvalidType_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockTenantRepository()
	createUseCase := usecase.NewCreateTenantUseCase(mockRepo, sharedmetrics.NoopRecorder{})
	ctx := context.Background()

	ownerID := uuid.New()
	req := &request.CreateTenantRequest{
		Name:        "Tenant Invalido",
		Slug:        "tenant-invalido",
		Description: "Descripcion del tenant",
		Type:        "INVALID_TYPE",
		OwnerID:     ownerID.String(),
	}

	// Act
	resp, err := createUseCase.Execute(ctx, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, exception.ErrInvalidTenantType, err)
}

func TestCreateTenantUseCase_Execute_InvalidOwnerID_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockTenantRepository()
	createUseCase := usecase.NewCreateTenantUseCase(mockRepo, sharedmetrics.NoopRecorder{})
	ctx := context.Background()

	req := &request.CreateTenantRequest{
		Name:        "Tenant",
		Slug:        "tenant",
		Description: "Descripcion del tenant",
		Type:        "PERSONAL",
		OwnerID:     "not-a-uuid",
	}

	// Act
	resp, err := createUseCase.Execute(ctx, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, exception.ErrInvalidOwner, err)
}

func TestCreateTenantUseCase_Execute_WithDomain_SetsDomainCorrectly(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockTenantRepository()
	createUseCase := usecase.NewCreateTenantUseCase(mockRepo, sharedmetrics.NoopRecorder{})
	ctx := context.Background()

	ownerID := uuid.New()
	req := &request.CreateTenantRequest{
		Name:        "Tenant con Dominio",
		Slug:        "tenant-dominio",
		Description: "Tiene dominio personalizado",
		Type:        "BUSINESS",
		Domain:      "custom.domain.com",
		OwnerID:     ownerID.String(),
	}

	// Act
	resp, err := createUseCase.Execute(ctx, req)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "custom.domain.com", resp.Domain)
	assert.True(t, resp.HasDomain)
	assert.Equal(t, 1, mockRepo.GetCallCount("ExistsByDomain"))
}

func TestCreateTenantUseCase_Execute_RepoCreateFails_ReturnsError(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockTenantRepository()
	createUseCase := usecase.NewCreateTenantUseCase(mockRepo, sharedmetrics.NoopRecorder{})
	ctx := context.Background()

	mockRepo.ShouldFailOn("Create")

	ownerID := uuid.New()
	req := &request.CreateTenantRequest{
		Name:        "Tenant",
		Slug:        "tenant",
		Description: "Descripcion del tenant",
		Type:        "PERSONAL",
		OwnerID:     ownerID.String(),
	}

	// Act
	resp, err := createUseCase.Execute(ctx, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, repository.ErrMockFailedOp, err)
}
