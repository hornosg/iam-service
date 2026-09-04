package entity

import (
	"iam/src/access/domain/value_object"
	"time"

	"github.com/google/uuid"
)

// ACC-E02 T11: `roles` es un catálogo global (T1-D5 / T10) — la entidad ya no
// conoce tenant.
type Role struct {
	ID          uuid.UUID
	Name        string
	Description string
	Type        value_object.RoleType
	Permissions []string
	IsActive    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func NewRole(name, description string, roleType value_object.RoleType) *Role {
	return &Role{
		ID:          uuid.New(),
		Name:        name,
		Description: description,
		Type:        roleType,
		Permissions: []string{},
		IsActive:    true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

func (r *Role) UpdateDetails(name, description string) {
	r.Name = name
	r.Description = description
	r.UpdatedAt = time.Now()
}

func (r *Role) Activate() {
	r.IsActive = true
	r.UpdatedAt = time.Now()
}

func (r *Role) Deactivate() {
	r.IsActive = false
	r.UpdatedAt = time.Now()
}

func (r *Role) AddPermission(permission string) {
	if !r.HasPermission(permission) {
		r.Permissions = append(r.Permissions, permission)
		r.UpdatedAt = time.Now()
	}
}

func (r *Role) RemovePermission(permission string) {
	for i, p := range r.Permissions {
		if p == permission {
			r.Permissions = append(r.Permissions[:i], r.Permissions[i+1:]...)
			r.UpdatedAt = time.Now()
			break
		}
	}
}

func (r *Role) HasPermission(permission string) bool {
	for _, p := range r.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}

func (r *Role) IsSystemRole() bool {
	return r.Type == value_object.RoleTypeSystemAdmin
}

func (r *Role) IsTenantRole() bool {
	return !r.IsSystemRole()
}

func (r *Role) CanManageUsers() bool {
	return r.Type.CanManageUsers()
}

func (r *Role) CanManageTenant() bool {
	return r.Type.CanManageTenant()
}
