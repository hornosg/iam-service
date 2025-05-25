package value_object

import "fmt"

type PlanStatus string

const (
	PlanStatusActive     PlanStatus = "ACTIVE"
	PlanStatusInactive   PlanStatus = "INACTIVE"
	PlanStatusDeprecated PlanStatus = "DEPRECATED"
)

func NewPlanStatusFromString(s string) (PlanStatus, error) {
	status := PlanStatus(s)
	if !status.IsValid() {
		return "", fmt.Errorf("invalid plan status: %s", s)
	}
	return status, nil
}

func (p PlanStatus) IsValid() bool {
	switch p {
	case PlanStatusActive, PlanStatusInactive, PlanStatusDeprecated:
		return true
	default:
		return false
	}
}

func (p PlanStatus) String() string {
	return string(p)
}

func (p PlanStatus) IsActive() bool {
	return p == PlanStatusActive
}

func (p PlanStatus) CanBeAssigned() bool {
	return p == PlanStatusActive
}
