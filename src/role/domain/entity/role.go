package entity

import (
	"iam/src/role/domain/value_object"
	"time"

	"github.com/google/uuid"
)

type Role struct {
	ID          uuid.UUID
	Name        string
	Description string
	Type        value_object.RoleType
	TenantID    *uuid.UUID // nil para roles de sistema
	Permissions []string
	IsActive    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func NewRole(name, description string, roleType value_object.RoleType, tenantID *uuid.UUID) *Role {
	return &Role{
		ID:          uuid.New(),
		Name:        name,
		Description: description,
		Type:        roleType,
		TenantID:    tenantID,
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
	return r.TenantID == nil
}

func (r *Role) IsTenantRole() bool {
	return r.TenantID != nil
}

func (r *Role) CanManageUsers() bool {
	return r.Type.CanManageUsers()
}

func (r *Role) CanManageTenant() bool {
	return r.Type.CanManageTenant()
}
