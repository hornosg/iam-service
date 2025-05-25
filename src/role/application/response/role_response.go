package response

import (
	"iam/src/role/domain/entity"
	"time"

	"github.com/google/uuid"
)

type RoleResponse struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Type        string     `json:"type"`
	TenantID    *uuid.UUID `json:"tenant_id,omitempty"`
	Permissions []string   `json:"permissions"`
	IsActive    bool       `json:"is_active"`
	IsSystem    bool       `json:"is_system"`
	IsTenant    bool       `json:"is_tenant"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type RoleListResponse struct {
	Roles      []*RoleResponse `json:"roles"`
	TotalCount int             `json:"total_count"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
}

func NewRoleResponse(role *entity.Role) *RoleResponse {
	return &RoleResponse{
		ID:          role.ID,
		Name:        role.Name,
		Description: role.Description,
		Type:        role.Type.String(),
		TenantID:    role.TenantID,
		Permissions: role.Permissions,
		IsActive:    role.IsActive,
		IsSystem:    role.IsSystemRole(),
		IsTenant:    role.IsTenantRole(),
		CreatedAt:   role.CreatedAt,
		UpdatedAt:   role.UpdatedAt,
	}
}

func NewRoleListResponse(roles []*entity.Role, totalCount, page, pageSize int) *RoleListResponse {
	roleResponses := make([]*RoleResponse, len(roles))
	for i, role := range roles {
		roleResponses[i] = NewRoleResponse(role)
	}

	return &RoleListResponse{
		Roles:      roleResponses,
		TotalCount: totalCount,
		Page:       page,
		PageSize:   pageSize,
	}
}
