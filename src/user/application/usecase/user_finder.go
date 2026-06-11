package usecase

import (
	"context"
	"github.com/hornosg/go-shared/domain/service"
	"iam/src/user/domain/port"

	"github.com/google/uuid"
)

type UserFinderUseCase struct {
	userRepo port.UserRepository
}

func NewUserFinderUseCase(userRepo port.UserRepository) *UserFinderUseCase {
	return &UserFinderUseCase{
		userRepo: userRepo,
	}
}

// FindUserByEmail implementa service.UserFinderService
func (uc *UserFinderUseCase) FindUserByEmail(ctx context.Context, email string, tenantID *uuid.UUID) (*service.BasicUserData, error) {
	user, err := uc.userRepo.GetByEmail(ctx, email, tenantID)
	if err != nil {
		return nil, err
	}

	return &service.BasicUserData{
		ID:           user.ID,
		Email:        user.Email.Value(),
		PasswordHash: user.PasswordHash,
		TenantID:     user.TenantID,
		RoleID:       user.RoleID,
		Status:       user.Status.String(),
		Provider:     user.Provider,
		FederatedID:  user.FederatedID,
	}, nil
}

// FindUserByID implementa service.UserFinderService
func (uc *UserFinderUseCase) FindUserByID(ctx context.Context, id uuid.UUID) (*service.BasicUserData, error) {
	user, err := uc.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return &service.BasicUserData{
		ID:           user.ID,
		Email:        user.Email.Value(),
		PasswordHash: user.PasswordHash,
		TenantID:     user.TenantID,
		RoleID:       user.RoleID,
		Status:       user.Status.String(),
		Provider:     user.Provider,
		FederatedID:  user.FederatedID,
	}, nil
}

// Verificar que implementa la interfaz en tiempo de compilación
var _ service.UserFinderService = (*UserFinderUseCase)(nil)
