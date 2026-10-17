package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"github.com/hornosg/iam-service/src/tenancy/domain/entity"
)

// MockTenantConfigRepository es un mock del repositorio de config (mudado de tenant-service,
// ACC-E04 T3) con la operación insert-only del bootstrap.
type MockTenantConfigRepository struct {
	mock.Mock
}

func (m *MockTenantConfigRepository) GetByKey(ctx context.Context, tenantID uuid.UUID, key string) (*entity.TenantConfig, bool, error) {
	args := m.Called(ctx, tenantID, key)
	if args.Get(0) == nil {
		return nil, args.Bool(1), args.Error(2)
	}
	return args.Get(0).(*entity.TenantConfig), args.Bool(1), args.Error(2)
}

func (m *MockTenantConfigRepository) Save(ctx context.Context, config *entity.TenantConfig) error {
	args := m.Called(ctx, config)
	return args.Error(0)
}

func (m *MockTenantConfigRepository) InsertMissing(ctx context.Context, tenantID uuid.UUID, configs []*entity.TenantConfig) (int, error) {
	args := m.Called(ctx, tenantID, configs)
	return args.Int(0), args.Error(1)
}
