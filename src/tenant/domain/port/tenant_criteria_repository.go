package port

import (
	"github.com/mercadocercano/criteria"
	"iam/src/tenant/domain/entity"
)

// TenantCriteriaRepository extiende TenantRepository con soporte para criteria
type TenantCriteriaRepository interface {
	TenantRepository
	criteria.CriteriaRepository[entity.Tenant]
}
