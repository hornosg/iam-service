package exception

import "errors"

var (
	ErrRoleNotFound           = errors.New("role not found")
	ErrRoleAlreadyExists      = errors.New("role already exists")
	ErrInvalidRoleType        = errors.New("invalid role type")
	ErrRoleNotActive          = errors.New("role is not active")
	ErrCannotDeleteRole       = errors.New("cannot delete role")
	ErrPermissionNotFound     = errors.New("permission not found")
	ErrInvalidTenant          = errors.New("invalid tenant for role")
	ErrSystemRoleModification = errors.New("cannot modify system role")
)
