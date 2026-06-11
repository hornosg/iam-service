package port

import (
	"github.com/hornosg/go-shared/criteria"
	"iam/src/tenant/domain/entity"
)

// TenantCriteriaRepository extiende TenantRepository con soporte para criteria
type TenantCriteriaRepository interface {
	TenantRepository
	criteria.CriteriaRepository[entity.Tenant]
}
