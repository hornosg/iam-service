package request

import (
	"iam/src/tenant/domain/value_object"
	"strings"

	"github.com/google/uuid"
)

type CreateTenantRequest struct {
	Name        string `json:"name" binding:"required,min=2,max=100"`
	Slug        string `json:"slug" binding:"required,min=2,max=50,alphanum"`
	Description string `json:"description" binding:"required,min=5,max=500"`
	Type        string `json:"type" binding:"required,oneof=PERSONAL STARTUP BUSINESS ENTERPRISE"`
	Domain      string `json:"domain,omitempty" binding:"omitempty,fqdn"`
	OwnerID     string `json:"owner_id" binding:"required,uuid"`
}

func (r *CreateTenantRequest) GetTenantType() (value_object.TenantType, error) {
	return value_object.NewTenantTypeFromString(r.Type)
}

func (r *CreateTenantRequest) GetOwnerID() (uuid.UUID, error) {
	return uuid.Parse(r.OwnerID)
}

func (r *CreateTenantRequest) GetNormalizedSlug() string {
	return strings.ToLower(strings.TrimSpace(r.Slug))
}

func (r *CreateTenantRequest) GetNormalizedDomain() string {
	if r.Domain == "" {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(r.Domain))
}
