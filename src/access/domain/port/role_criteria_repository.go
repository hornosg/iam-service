package port

import (
	"github.com/hornosg/go-shared/criteria"
	"github.com/hornosg/iam-service/src/access/domain/entity"
)

// RoleCriteriaRepository extiende RoleRepository con soporte para criteria
type RoleCriteriaRepository interface {
	RoleRepository
	criteria.CriteriaRepository[entity.Role]
}
