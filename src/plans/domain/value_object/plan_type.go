package value_object

import "fmt"

type PlanType string

const (
	PlanTypeFree       PlanType = "FREE"
	PlanTypeBasic      PlanType = "BASIC"
	PlanTypePremium    PlanType = "PREMIUM"
	PlanTypeEnterprise PlanType = "ENTERPRISE"
)

func NewPlanTypeFromString(s string) (PlanType, error) {
	planType := PlanType(s)
	if !planType.IsValid() {
		return "", fmt.Errorf("invalid plan type: %s", s)
	}
	return planType, nil
}

func (p PlanType) IsValid() bool {
	switch p {
	case PlanTypeFree, PlanTypeBasic, PlanTypePremium, PlanTypeEnterprise:
		return true
	default:
		return false
	}
}

func (p PlanType) String() string {
	return string(p)
}

func (p PlanType) IsFree() bool {
	return p == PlanTypeFree
}

func (p PlanType) AllowsMultipleUsers() bool {
	return p != PlanTypeFree
}

func (p PlanType) GetMaxUsers() int {
	switch p {
	case PlanTypeFree:
		return 1
	case PlanTypeBasic:
		return 10
	case PlanTypePremium:
		return 100
	case PlanTypeEnterprise:
		return -1 // Unlimited
	default:
		return 0
	}
}
