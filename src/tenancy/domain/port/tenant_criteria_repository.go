package port

import (
	"github.com/hornosg/go-shared/criteria"
	"github.com/hornosg/iam-service/src/tenancy/domain/entity"
)

// TenantCriteriaRepository extiende TenantRepository con soporte para criteria
type TenantCriteriaRepository interface {
	TenantRepository
	criteria.CriteriaRepository[entity.Tenant]
}
