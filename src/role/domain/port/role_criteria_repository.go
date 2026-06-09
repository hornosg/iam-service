package port

import (
	"github.com/mercadocercano/criteria"
	"iam/src/role/domain/entity"
)

// RoleCriteriaRepository extiende RoleRepository con soporte para criteria
type RoleCriteriaRepository interface {
	RoleRepository
	criteria.CriteriaRepository[entity.Role]
}
