package value_object

import "fmt"

type RoleType string

const (
	RoleTypeSystemAdmin RoleType = "SYSTEM_ADMIN" // Administrador del sistema
	RoleTypeTenantAdmin RoleType = "TENANT_ADMIN" // Administrador del tenant
	RoleTypeUser        RoleType = "USER"         // Usuario regular
	RoleTypeReadOnly    RoleType = "READ_ONLY"    // Solo lectura
	RoleTypeCustom      RoleType = "CUSTOM"       // Rol personalizado
)

func NewRoleTypeFromString(s string) (RoleType, error) {
	roleType := RoleType(s)
	if !roleType.IsValid() {
		return "", fmt.Errorf("invalid role type: %s", s)
	}
	return roleType, nil
}

func (r RoleType) IsValid() bool {
	switch r {
	case RoleTypeSystemAdmin, RoleTypeTenantAdmin, RoleTypeUser, RoleTypeReadOnly, RoleTypeCustom:
		return true
	default:
		return false
	}
}

func (r RoleType) String() string {
	return string(r)
}

func (r RoleType) IsSystemLevel() bool {
	return r == RoleTypeSystemAdmin
}

func (r RoleType) IsTenantLevel() bool {
	return r == RoleTypeTenantAdmin || r == RoleTypeUser || r == RoleTypeReadOnly || r == RoleTypeCustom
}

func (r RoleType) CanManageUsers() bool {
	return r == RoleTypeSystemAdmin || r == RoleTypeTenantAdmin
}

func (r RoleType) CanManageTenant() bool {
	return r == RoleTypeSystemAdmin || r == RoleTypeTenantAdmin
}
