package request

import (
	"iam/src/role/domain/value_object"

	"github.com/google/uuid"
)

type CreateRoleRequest struct {
	Name        string   `json:"name" binding:"required,min=2,max=100"`
	Description string   `json:"description" binding:"required,min=5,max=500"`
	Type        string   `json:"type" binding:"required,oneof=SYSTEM_ADMIN TENANT_ADMIN USER READ_ONLY CUSTOM"`
	TenantID    *string  `json:"tenant_id,omitempty"` // Opcional, para roles de tenant
	Permissions []string `json:"permissions,omitempty"`
}

func (r *CreateRoleRequest) GetRoleType() (value_object.RoleType, error) {
	return value_object.NewRoleTypeFromString(r.Type)
}

func (r *CreateRoleRequest) GetTenantID() (*uuid.UUID, error) {
	if r.TenantID == nil {
		return nil, nil
	}

	tenantID, err := uuid.Parse(*r.TenantID)
	if err != nil {
		return nil, err
	}

	return &tenantID, nil
}
