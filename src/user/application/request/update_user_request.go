package request

import (
	"iam/src/user/domain/value_object"

	"github.com/google/uuid"
)

type UpdateUserRequest struct {
	ID     uuid.UUID                `json:"id" binding:"required"`
	Email  *string                  `json:"email,omitempty"`
	RoleID *uuid.UUID               `json:"role_id,omitempty"`
	Status *value_object.UserStatus `json:"status,omitempty"`
}

func (r *UpdateUserRequest) HasEmailUpdate() bool {
	return r.Email != nil && *r.Email != ""
}

func (r *UpdateUserRequest) HasRoleUpdate() bool {
	return r.RoleID != nil && *r.RoleID != uuid.Nil
}

func (r *UpdateUserRequest) HasStatusUpdate() bool {
	return r.Status != nil && r.Status.IsValid()
}

func (r *UpdateUserRequest) ToEmail() (*value_object.Email, error) {
	if !r.HasEmailUpdate() {
		return nil, nil
	}
	return value_object.NewEmail(*r.Email)
}
