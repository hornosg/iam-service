package request

import (
	"errors"
	"iam/src/user/domain/value_object"

	"github.com/google/uuid"
)

type CreateUserRequest struct {
	Email    string    `json:"email" binding:"required,email"`
	Password string    `json:"password,omitempty"`
	TenantID uuid.UUID `json:"tenant_id" binding:"required"`
	RoleID   uuid.UUID `json:"role_id" binding:"required"`
	Provider string    `json:"provider,omitempty"`
}

func (r *CreateUserRequest) Validate() error {
	if r.Email == "" {
		return errors.New("email es requerido")
	}

	if r.Provider == "" || r.Provider == "LOCAL" {
		if r.Password == "" {
			return errors.New("password es requerido para usuarios locales")
		}
		if len(r.Password) < 8 {
			return errors.New("password debe tener al menos 8 caracteres")
		}
	}

	if r.TenantID == uuid.Nil {
		return errors.New("tenant_id es requerido")
	}

	if r.RoleID == uuid.Nil {
		return errors.New("role_id es requerido")
	}

	return nil
}

func (r *CreateUserRequest) ToEmail() (*value_object.Email, error) {
	return value_object.NewEmail(r.Email)
}
