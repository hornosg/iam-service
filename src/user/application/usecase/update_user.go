package usecase

import (
	"context"
	"iam/src/user/application/request"
	"iam/src/user/application/response"
	"iam/src/user/domain/exception"
	"iam/src/user/domain/port"
)

type UpdateUserUseCase struct {
	userRepo port.UserRepository
}

func NewUpdateUserUseCase(userRepo port.UserRepository) *UpdateUserUseCase {
	return &UpdateUserUseCase{
		userRepo: userRepo,
	}
}

func (uc *UpdateUserUseCase) Execute(ctx context.Context, req *request.UpdateUserRequest) (*response.UserResponse, error) {
	// Obtener usuario existente
	user, err := uc.userRepo.GetByID(ctx, req.ID)
	if err != nil {
		return nil, exception.ErrUserNotFound
	}

	// Actualizar email si se proporciona
	if req.HasEmailUpdate() {
		email, err := req.ToEmail()
		if err != nil {
			return nil, exception.ErrInvalidEmail
		}

		// Verificar que el nuevo email no esté en uso
		exists, err := uc.userRepo.ExistsByEmail(ctx, *req.Email, &user.TenantID)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, exception.ErrUserAlreadyExists
		}

		user.UpdateEmail(email)
	}

	// Actualizar role si se proporciona
	if req.HasRoleUpdate() {
		user.RoleID = *req.RoleID
	}

	// Actualizar status si se proporciona
	if req.HasStatusUpdate() {
		if err := user.ChangeStatus(*req.Status); err != nil {
			return nil, err
		}
	}

	// Guardar cambios
	if err := uc.userRepo.Update(ctx, user); err != nil {
		return nil, err
	}

	return response.NewUserResponse(user), nil
}
