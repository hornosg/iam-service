package usecase

import (
	"context"

	"github.com/hornosg/go-shared/criteria"
	"iam/src/user/application/response"
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
func (uc *ListUsersByCriteriaUseCase) Execute(ctx context.Context, searchCriteria criteria.Criteria) (*criteria.ListResponse[response.UserResponse], error) {
	users, err := uc.userRepo.SearchByCriteria(ctx, searchCriteria)
	if err != nil {
		return nil, err
	}

	total, err := uc.userRepo.CountByCriteria(ctx, searchCriteria)
	if err != nil {
		return nil, err
	}

	dtos := make([]*response.UserResponse, len(users))
	for i, u := range users {
		dtos[i] = response.NewUserResponse(u)
	}
	return criteria.NewListResponseFromCriteria(dtos, total, searchCriteria), nil
}
