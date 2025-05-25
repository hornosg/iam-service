package request

import (
	"iam/src/plan/domain/value_object"
)

type CreatePlanRequest struct {
	Name        string   `json:"name" binding:"required,min=2,max=100"`
	Description string   `json:"description" binding:"required,min=10,max=500"`
	Type        string   `json:"type" binding:"required,oneof=FREE BASIC PREMIUM ENTERPRISE"`
	PriceMonth  float64  `json:"price_month" binding:"min=0"`
	PriceYear   float64  `json:"price_year" binding:"min=0"`
	Features    []string `json:"features,omitempty"`
}

func (r *CreatePlanRequest) GetPlanType() (value_object.PlanType, error) {
	return value_object.NewPlanTypeFromString(r.Type)
}
