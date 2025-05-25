package usecase

import (
	"context"

	"iam/src/shared/domain/criteria"
	"iam/src/user/domain/entity"
	"iam/src/user/domain/port"
)

// ListUsersByCriteriaUseCase lista usuarios usando el patrón criteria
type ListUsersByCriteriaUseCase struct {
	userRepo port.UserCriteriaRepository
}

// NewListUsersByCriteriaUseCase crea una nueva instancia del UseCase
func NewListUsersByCriteriaUseCase(userRepo port.UserCriteriaRepository) *ListUsersByCriteriaUseCase {
	return &ListUsersByCriteriaUseCase{
		userRepo: userRepo,
	}
}

// Execute ejecuta la búsqueda de usuarios por criterios
func (uc *ListUsersByCriteriaUseCase) Execute(ctx context.Context, searchCriteria criteria.Criteria) (*criteria.ListResponse[entity.User], error) {
	// Buscar usuarios según criterios
	users, err := uc.userRepo.SearchByCriteria(ctx, searchCriteria)
	if err != nil {
		return nil, err
	}

	// Contar total de usuarios según criterios (sin paginación)
	total, err := uc.userRepo.CountByCriteria(ctx, searchCriteria)
	if err != nil {
		return nil, err
	}

	// Crear respuesta usando el helper genérico
	return criteria.NewListResponse(users, total, searchCriteria), nil
}
