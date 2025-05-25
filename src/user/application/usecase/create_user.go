package usecase

import (
	"context"
	"iam/src/user/application/request"
	"iam/src/user/application/response"
	"iam/src/user/domain/entity"
	"iam/src/user/domain/exception"
	"iam/src/user/domain/port"
)

type CreateUserUseCase struct {
	userRepo port.UserRepository
}

func NewCreateUserUseCase(userRepo port.UserRepository) *CreateUserUseCase {
	return &CreateUserUseCase{
		userRepo: userRepo,
	}
}

func (uc *CreateUserUseCase) Execute(ctx context.Context, req *request.CreateUserRequest) (*response.UserResponse, error) {
	// Validar request
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Verificar que el email no exista
	exists, err := uc.userRepo.ExistsByEmail(ctx, req.Email, &req.TenantID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, exception.ErrUserAlreadyExists
	}

	// Crear email value object
	email, err := req.ToEmail()
	if err != nil {
		return nil, exception.ErrInvalidEmail
	}

	// Crear nueva entidad usuario
	user := entity.NewUser(email, req.TenantID, req.RoleID)

	// Configurar provider si se especifica
	if req.Provider != "" && req.Provider != "LOCAL" {
		user.Provider = req.Provider
	}

	// Establecer password si es usuario local
	if req.Password != "" {
		if err := user.SetPassword(req.Password); err != nil {
			return nil, err
		}
	}

	// Guardar en repositorio
	if err := uc.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}

	return response.NewUserResponse(user), nil
}
