package request

import (
	"strings"

	"github.com/google/uuid"
)

type UpdateTenantRequest struct {
	Name        *string `json:"name,omitempty" binding:"omitempty,min=2,max=100"`
	Description *string `json:"description,omitempty" binding:"omitempty,min=5,max=500"`
	Domain      *string `json:"domain,omitempty" binding:"omitempty,fqdn"`
	Status      *string `json:"status,omitempty" binding:"omitempty,oneof=ACTIVE INACTIVE SUSPENDED"`
	MaxUsers    *int    `json:"max_users,omitempty" binding:"omitempty,min=-1"`
}

type SetPlanRequest struct {
	PlanID string `json:"plan_id" binding:"required,uuid"`
}

func (r *SetPlanRequest) GetPlanID() (uuid.UUID, error) {
	return uuid.Parse(r.PlanID)
}

func (r *UpdateTenantRequest) GetNormalizedDomain() *string {
	if r.Domain == nil || *r.Domain == "" {
		return nil
	}
	normalized := strings.ToLower(strings.TrimSpace(*r.Domain))
	return &normalized
}
