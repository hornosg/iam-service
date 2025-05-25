package port

import (
	"context"
	"iam/src/shared/domain/criteria"
	"iam/src/user/domain/entity"
	"iam/src/user/domain/value_object"

	"github.com/google/uuid"
)

type UserRepository interface {
	// CRUD básico
	Create(ctx context.Context, user *entity.User) error
	GetByID(ctx context.Context, id uuid.UUID) (*entity.User, error)
	GetByEmail(ctx context.Context, email string, tenantID *uuid.UUID) (*entity.User, error)
	Update(ctx context.Context, user *entity.User) error
	Delete(ctx context.Context, id uuid.UUID) error

	// Búsquedas específicas
	GetByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]*entity.User, error)
	GetByStatus(ctx context.Context, status value_object.UserStatus, limit, offset int) ([]*entity.User, error)
	GetByRole(ctx context.Context, roleID uuid.UUID, limit, offset int) ([]*entity.User, error)

	// Verificaciones
	ExistsByEmail(ctx context.Context, email string, tenantID *uuid.UUID) (bool, error)
	CountByTenant(ctx context.Context, tenantID uuid.UUID) (int, error)
	CountByStatus(ctx context.Context, status value_object.UserStatus) (int, error)
}

// UserCriteriaRepository extiende UserRepository con capacidades de criterios
type UserCriteriaRepository interface {
	UserRepository
	criteria.CriteriaRepository[entity.User]
}
