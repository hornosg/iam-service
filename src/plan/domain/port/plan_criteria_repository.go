package port

import (
	"github.com/hornosg/go-shared/criteria"
	"iam/src/plan/domain/entity"
)

// PlanCriteriaRepository extiende PlanRepository con soporte para criteria
type PlanCriteriaRepository interface {
	PlanRepository
	criteria.CriteriaRepository[entity.Plan]
}
