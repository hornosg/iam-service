package response

import (
	"iam/src/user/domain/entity"
	"iam/src/user/domain/value_object"
	"time"

	"github.com/google/uuid"
)

type UserResponse struct {
	ID          uuid.UUID               `json:"id"`
	Email       string                  `json:"email"`
	TenantID    uuid.UUID               `json:"tenant_id"`
	RoleID      uuid.UUID               `json:"role_id"`
	Status      value_object.UserStatus `json:"status"`
	Provider    string                  `json:"provider"`
	FederatedID string                  `json:"federated_id,omitempty"`
	CreatedAt   time.Time               `json:"created_at"`
	UpdatedAt   time.Time               `json:"updated_at"`
}

func NewUserResponse(user *entity.User) *UserResponse {
	return &UserResponse{
		ID:          user.ID,
		Email:       user.Email.Value(),
		TenantID:    user.TenantID,
		RoleID:      user.RoleID,
		Status:      user.Status,
		Provider:    user.Provider,
		FederatedID: user.FederatedID,
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
	}
}

type UserListResponse struct {
	Users      []*UserResponse `json:"users"`
	Total      int             `json:"total"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	TotalPages int             `json:"total_pages"`
}

func NewUserListResponse(users []*entity.User, total, page, pageSize int) *UserListResponse {
	userResponses := make([]*UserResponse, len(users))
	for i, user := range users {
		userResponses[i] = NewUserResponse(user)
	}

	totalPages := (total + pageSize - 1) / pageSize

	return &UserListResponse{
		Users:      userResponses,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}
