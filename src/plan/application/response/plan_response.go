package response

import (
	"iam/src/plan/domain/entity"
	"iam/src/plan/domain/value_object"
	"time"

	"github.com/google/uuid"
)

type PlanResponse struct {
	ID             uuid.UUID               `json:"id"`
	Name           string                  `json:"name"`
	Description    string                  `json:"description"`
	Type           string                  `json:"type"`
	Status         string                  `json:"status"`
	MaxUsers       int                     `json:"max_users"`
	PriceMonth     float64                 `json:"price_month"`
	PriceYear      float64                 `json:"price_year"`
	YearlyDiscount float64                 `json:"yearly_discount"`
	Features       []string                `json:"features"`
	RateLimits     value_object.RateLimits `json:"rate_limits"`
	CreatedAt      time.Time               `json:"created_at"`
	UpdatedAt      time.Time               `json:"updated_at"`
}

type PlanListResponse struct {
	Plans      []*PlanResponse `json:"plans"`
	TotalCount int             `json:"total_count"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
}

func NewPlanResponse(plan *entity.Plan) *PlanResponse {
	return &PlanResponse{
		ID:             plan.ID,
		Name:           plan.Name,
		Description:    plan.Description,
		Type:           plan.Type.String(),
		Status:         plan.Status.String(),
		MaxUsers:       plan.MaxUsers,
		PriceMonth:     plan.PriceMonth,
		PriceYear:      plan.PriceYear,
		YearlyDiscount: plan.GetYearlyDiscount(),
		Features:       plan.Features,
		RateLimits:     plan.RateLimits,
		CreatedAt:      plan.CreatedAt,
		UpdatedAt:      plan.UpdatedAt,
	}
}

func NewPlanListResponse(plans []*entity.Plan, totalCount, page, pageSize int) *PlanListResponse {
	planResponses := make([]*PlanResponse, len(plans))
	for i, plan := range plans {
		planResponses[i] = NewPlanResponse(plan)
	}

	return &PlanListResponse{
		Plans:      planResponses,
		TotalCount: totalCount,
		Page:       page,
		PageSize:   pageSize,
	}
}
