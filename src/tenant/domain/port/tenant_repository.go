package port

import (
	"context"
	"iam/src/tenant/domain/entity"
	"iam/src/tenant/domain/value_object"

	"github.com/google/uuid"
)

type TenantRepository interface {
	// CRUD básico
	Create(ctx context.Context, tenant *entity.Tenant) error
	GetByID(ctx context.Context, id uuid.UUID) (*entity.Tenant, error)
	GetBySlug(ctx context.Context, slug string) (*entity.Tenant, error)
	GetByDomain(ctx context.Context, domain string) (*entity.Tenant, error)
	Update(ctx context.Context, tenant *entity.Tenant) error
	Delete(ctx context.Context, id uuid.UUID) error

	// Búsquedas específicas
	GetByOwner(ctx context.Context, ownerID uuid.UUID) ([]*entity.Tenant, error)
	GetByStatus(ctx context.Context, status value_object.TenantStatus, limit, offset int) ([]*entity.Tenant, error)
	GetByType(ctx context.Context, tenantType value_object.TenantType, limit, offset int) ([]*entity.Tenant, error)
	GetByPlan(ctx context.Context, planID uuid.UUID, limit, offset int) ([]*entity.Tenant, error)
	GetActive(ctx context.Context, limit, offset int) ([]*entity.Tenant, error)
	GetExpiring(ctx context.Context, days int) ([]*entity.Tenant, error)
	List(ctx context.Context, limit, offset int) ([]*entity.Tenant, error)

	// Verificaciones
	ExistsBySlug(ctx context.Context, slug string) (bool, error)
	ExistsByDomain(ctx context.Context, domain string) (bool, error)
	Count(ctx context.Context) (int, error)
	CountByStatus(ctx context.Context, status value_object.TenantStatus) (int, error)
	CountByOwner(ctx context.Context, ownerID uuid.UUID) (int, error)
	CountByPlan(ctx context.Context, planID uuid.UUID) (int, error)
}
