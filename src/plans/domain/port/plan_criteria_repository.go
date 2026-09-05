package port

import (
	"github.com/hornosg/go-shared/criteria"
	"github.com/hornosg/iam-service/src/plans/domain/entity"
)

// PlanCriteriaRepository extiende PlanRepository con soporte para criteria
type PlanCriteriaRepository interface {
	PlanRepository
	criteria.CriteriaRepository[entity.Plan]
}
