package request

type UpdatePlanRequest struct {
	Name        *string   `json:"name,omitempty" binding:"omitempty,min=2,max=100"`
	Description *string   `json:"description,omitempty" binding:"omitempty,min=10,max=500"`
	PriceMonth  *float64  `json:"price_month,omitempty" binding:"omitempty,min=0"`
	PriceYear   *float64  `json:"price_year,omitempty" binding:"omitempty,min=0"`
	Status      *string   `json:"status,omitempty" binding:"omitempty,oneof=ACTIVE INACTIVE DEPRECATED"`
	Features    *[]string `json:"features,omitempty"`
}
