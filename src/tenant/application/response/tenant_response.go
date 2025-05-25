package response

import (
	"iam/src/tenant/domain/entity"
	"iam/src/tenant/domain/value_object"
	"time"

	"github.com/google/uuid"
)

type TenantResponse struct {
	ID           uuid.UUID                    `json:"id"`
	Name         string                       `json:"name"`
	Slug         string                       `json:"slug"`
	Description  string                       `json:"description"`
	Type         string                       `json:"type"`
	Status       string                       `json:"status"`
	PlanID       *uuid.UUID                   `json:"plan_id,omitempty"`
	Domain       string                       `json:"domain,omitempty"`
	MaxUsers     int                          `json:"max_users"`
	UserCount    int                          `json:"user_count"`
	OwnerID      uuid.UUID                    `json:"owner_id"`
	Settings     map[string]interface{}       `json:"settings"`
	Features     *value_object.TenantFeatures `json:"features"`
	SubscribedAt *time.Time                   `json:"subscribed_at,omitempty"`
	ExpiresAt    *time.Time                   `json:"expires_at,omitempty"`
	IsActive     bool                         `json:"is_active"`
	CanAccess    bool                         `json:"can_access"`
	IsExpired    bool                         `json:"is_expired"`
	CanAddUser   bool                         `json:"can_add_user"`
	HasPlan      bool                         `json:"has_plan"`
	HasDomain    bool                         `json:"has_custom_domain"`
	CreatedAt    time.Time                    `json:"created_at"`
	UpdatedAt    time.Time                    `json:"updated_at"`
}

type TenantListResponse struct {
	Tenants    []*TenantResponse `json:"tenants"`
	TotalCount int               `json:"total_count"`
	Page       int               `json:"page"`
	PageSize   int               `json:"page_size"`
}

type TenantSummaryResponse struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Slug     string    `json:"slug"`
	Status   string    `json:"status"`
	Type     string    `json:"type"`
	IsActive bool      `json:"is_active"`
}

func NewTenantResponse(tenant *entity.Tenant) *TenantResponse {
	return &TenantResponse{
		ID:           tenant.ID,
		Name:         tenant.Name,
		Slug:         tenant.Slug,
		Description:  tenant.Description,
		Type:         tenant.Type.String(),
		Status:       tenant.Status.String(),
		PlanID:       tenant.PlanID,
		Domain:       tenant.Domain,
		MaxUsers:     tenant.MaxUsers,
		UserCount:    tenant.UserCount,
		OwnerID:      tenant.OwnerID,
		Settings:     tenant.Settings,
		Features:     tenant.GetFeatures(),
		SubscribedAt: tenant.SubscribedAt,
		ExpiresAt:    tenant.ExpiresAt,
		IsActive:     tenant.IsActive(),
		CanAccess:    tenant.CanAccess(),
		IsExpired:    tenant.IsExpired(),
		CanAddUser:   tenant.CanAddUser(),
		HasPlan:      tenant.HasPlan(),
		HasDomain:    tenant.HasCustomDomain(),
		CreatedAt:    tenant.CreatedAt,
		UpdatedAt:    tenant.UpdatedAt,
	}
}

func NewTenantSummaryResponse(tenant *entity.Tenant) *TenantSummaryResponse {
	return &TenantSummaryResponse{
		ID:       tenant.ID,
		Name:     tenant.Name,
		Slug:     tenant.Slug,
		Status:   tenant.Status.String(),
		Type:     tenant.Type.String(),
		IsActive: tenant.IsActive(),
	}
}

func NewTenantListResponse(tenants []*entity.Tenant, totalCount, page, pageSize int) *TenantListResponse {
	tenantResponses := make([]*TenantResponse, len(tenants))
	for i, tenant := range tenants {
		tenantResponses[i] = NewTenantResponse(tenant)
	}

	return &TenantListResponse{
		Tenants:    tenantResponses,
		TotalCount: totalCount,
		Page:       page,
		PageSize:   pageSize,
	}
}
