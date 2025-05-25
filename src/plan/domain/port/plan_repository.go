package port

import (
	"context"
	"iam/src/plan/domain/entity"
	"iam/src/plan/domain/value_object"

	"github.com/google/uuid"
)

type PlanRepository interface {
	// CRUD básico
	Create(ctx context.Context, plan *entity.Plan) error
	GetByID(ctx context.Context, id uuid.UUID) (*entity.Plan, error)
	GetByName(ctx context.Context, name string) (*entity.Plan, error)
	Update(ctx context.Context, plan *entity.Plan) error
	Delete(ctx context.Context, id uuid.UUID) error

	// Búsquedas específicas
	GetByType(ctx context.Context, planType value_object.PlanType) ([]*entity.Plan, error)
	GetByStatus(ctx context.Context, status value_object.PlanStatus) ([]*entity.Plan, error)
	GetActive(ctx context.Context) ([]*entity.Plan, error)
	List(ctx context.Context, limit, offset int) ([]*entity.Plan, error)

	// Verificaciones
	ExistsByName(ctx context.Context, name string) (bool, error)
	Count(ctx context.Context) (int, error)
	CountByStatus(ctx context.Context, status value_object.PlanStatus) (int, error)
}
