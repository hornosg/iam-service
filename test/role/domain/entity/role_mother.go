package entity

import (
	"time"

	"iam/src/role/domain/entity"
	"iam/src/role/domain/value_object"

	"github.com/google/uuid"
)

// RoleMother implementa el patrón Object Mother para crear entities Role de prueba
type RoleMother struct{}

// WithDefaults crea un rol con valores por defecto
func (RoleMother) WithDefaults() *entity.Role {
	now := time.Now()

	return &entity.Role{
		ID:          uuid.New(),
		Name:        "Rol de Prueba",
		Description: "Descripción de prueba",
		Type:        value_object.RoleTypeUser,
		TenantID:    nil, // Por defecto es rol de sistema
		Permissions: []string{"read:basic"},
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// WithID crea un rol con un ID específico
func (r RoleMother) WithID(id uuid.UUID) *entity.Role {
	role := r.WithDefaults()
	role.ID = id
	return role
}

// WithName crea un rol con un nombre específico
func (r RoleMother) WithName(name string) *entity.Role {
	role := r.WithDefaults()
	role.Name = name
	return role
}

// WithType crea un rol con un tipo específico
func (r RoleMother) WithType(roleType value_object.RoleType) *entity.Role {
	role := r.WithDefaults()
	role.Type = roleType
	return role
}

// WithTenant crea un rol asociado a un tenant específico
func (r RoleMother) WithTenant(tenantID uuid.UUID) *entity.Role {
	role := r.WithDefaults()
	role.TenantID = &tenantID
	role.Type = value_object.RoleTypeUser // Los roles de tenant suelen ser de usuario
	return role
}

// WithPermissions crea un rol con permisos específicos
func (r RoleMother) WithPermissions(permissions []string) *entity.Role {
	role := r.WithDefaults()
	role.Permissions = make([]string, len(permissions))
	copy(role.Permissions, permissions)
	return role
}

// SystemAdmin crea un rol de administrador del sistema
func (r RoleMother) SystemAdmin() *entity.Role {
	role := r.WithDefaults()
	role.Name = "System Administrator"
	role.Description = "Administrador del sistema con acceso completo"
	role.Type = value_object.RoleTypeSystemAdmin
	role.TenantID = nil
	role.Permissions = []string{
		"system:admin",
		"tenant:create",
		"tenant:update",
		"tenant:delete",
		"user:create",
		"user:update",
		"user:delete",
		"role:create",
		"role:update",
		"role:delete",
	}
	return role
}

// TenantAdmin crea un rol de administrador de tenant
func (r RoleMother) TenantAdmin() *entity.Role {
	tenantID := uuid.New()
	role := r.WithDefaults()
	role.Name = "Tenant Administrator"
	role.Description = "Administrador del tenant"
	role.Type = value_object.RoleTypeTenantAdmin
	role.TenantID = &tenantID
	role.Permissions = []string{
		"tenant:read",
		"tenant:update",
		"user:create",
		"user:update",
		"user:delete",
		"role:create",
		"role:update",
	}
	return role
}

// TenantAdminForTenant crea un rol de administrador para un tenant específico
func (r RoleMother) TenantAdminForTenant(tenantID uuid.UUID) *entity.Role {
	role := r.TenantAdmin()
	role.TenantID = &tenantID
	return role
}

// User crea un rol de usuario regular
func (r RoleMother) User() *entity.Role {
	tenantID := uuid.New()
	role := r.WithDefaults()
	role.Name = "User"
	role.Description = "Usuario regular"
	role.Type = value_object.RoleTypeUser
	role.TenantID = &tenantID
	role.Permissions = []string{
		"profile:read",
		"profile:update",
	}
	return role
}

// ReadOnly crea un rol de solo lectura
func (r RoleMother) ReadOnly() *entity.Role {
	tenantID := uuid.New()
	role := r.WithDefaults()
	role.Name = "Read Only"
	role.Description = "Solo lectura"
	role.Type = value_object.RoleTypeReadOnly
	role.TenantID = &tenantID
	role.Permissions = []string{
		"read:basic",
	}
	return role
}

// Custom crea un rol personalizado
func (r RoleMother) Custom() *entity.Role {
	tenantID := uuid.New()
	role := r.WithDefaults()
	role.Name = "Custom Role"
	role.Description = "Rol personalizado"
	role.Type = value_object.RoleTypeCustom
	role.TenantID = &tenantID
	role.Permissions = []string{
		"custom:action1",
		"custom:action2",
	}
	return role
}

// Inactive crea un rol inactivo
func (r RoleMother) Inactive() *entity.Role {
	role := r.WithDefaults()
	role.IsActive = false
	return role
}

// WithDescription crea un rol con una descripción específica
func (r RoleMother) WithDescription(description string) *entity.Role {
	role := r.WithDefaults()
	role.Description = description
	return role
}

// Complete crea un rol con todos los parámetros especificados
func (RoleMother) Complete(id uuid.UUID, name, description string, roleType value_object.RoleType,
	tenantID *uuid.UUID, permissions []string, isActive bool) *entity.Role {

	now := time.Now()
	permissionsCopy := make([]string, len(permissions))
	copy(permissionsCopy, permissions)

	return &entity.Role{
		ID:          id,
		Name:        name,
		Description: description,
		Type:        roleType,
		TenantID:    tenantID,
		Permissions: permissionsCopy,
		IsActive:    isActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// UserForTenant crea un rol de usuario para un tenant específico
func (r RoleMother) UserForTenant(tenantID uuid.UUID) *entity.Role {
	role := r.User()
	role.TenantID = &tenantID
	return role
}

// Create retorna una nueva instancia de RoleMother
func Create() RoleMother {
	return RoleMother{}
}
