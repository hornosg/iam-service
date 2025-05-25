package port

import (
	"context"
	"iam/src/role/domain/entity"
	"iam/src/role/domain/value_object"

	"github.com/google/uuid"
)

type RoleRepository interface {
	// CRUD básico
	Create(ctx context.Context, role *entity.Role) error
	GetByID(ctx context.Context, id uuid.UUID) (*entity.Role, error)
	GetByName(ctx context.Context, name string, tenantID *uuid.UUID) (*entity.Role, error)
	Update(ctx context.Context, role *entity.Role) error
	Delete(ctx context.Context, id uuid.UUID) error

	// Búsquedas específicas
	GetByType(ctx context.Context, roleType value_object.RoleType) ([]*entity.Role, error)
	GetByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]*entity.Role, error)
	GetSystemRoles(ctx context.Context) ([]*entity.Role, error)
	GetActiveRoles(ctx context.Context, tenantID *uuid.UUID, limit, offset int) ([]*entity.Role, error)
	List(ctx context.Context, limit, offset int) ([]*entity.Role, error)

	// Verificaciones
	ExistsByName(ctx context.Context, name string, tenantID *uuid.UUID) (bool, error)
	Count(ctx context.Context) (int, error)
	CountByTenant(ctx context.Context, tenantID uuid.UUID) (int, error)
	CountByType(ctx context.Context, roleType value_object.RoleType) (int, error)
}
