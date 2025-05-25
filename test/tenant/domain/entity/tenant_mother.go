package entity

import (
	"time"

	"iam/src/tenant/domain/entity"
	"iam/src/tenant/domain/value_object"

	"github.com/google/uuid"
)

// TenantMother implementa el patrón Object Mother para crear entities Tenant de prueba
type TenantMother struct{}

// WithDefaults crea un tenant con valores por defecto
func (TenantMother) WithDefaults() *entity.Tenant {
	now := time.Now()
	ownerID := uuid.New()

	return &entity.Tenant{
		ID:           uuid.New(),
		Name:         "Tenant de Prueba",
		Slug:         "tenant-prueba",
		Description:  "Descripción de prueba",
		Type:         value_object.TenantTypePersonal,
		Status:       value_object.TenantStatusActive,
		PlanID:       nil,
		Domain:       "",
		MaxUsers:     1,
		UserCount:    0,
		OwnerID:      ownerID,
		Settings:     make(map[string]interface{}),
		Features:     value_object.NewTenantFeatures(),
		SubscribedAt: nil,
		ExpiresAt:    nil,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// WithID crea un tenant con un ID específico
func (t TenantMother) WithID(id uuid.UUID) *entity.Tenant {
	tenant := t.WithDefaults()
	tenant.ID = id
	return tenant
}

// WithName crea un tenant con un nombre específico
func (t TenantMother) WithName(name string) *entity.Tenant {
	tenant := t.WithDefaults()
	tenant.Name = name
	return tenant
}

// WithSlug crea un tenant con un slug específico
func (t TenantMother) WithSlug(slug string) *entity.Tenant {
	tenant := t.WithDefaults()
	tenant.Slug = slug
	return tenant
}

// WithOwner crea un tenant con un propietario específico
func (t TenantMother) WithOwner(ownerID uuid.UUID) *entity.Tenant {
	tenant := t.WithDefaults()
	tenant.OwnerID = ownerID
	return tenant
}

// WithType crea un tenant con un tipo específico
func (t TenantMother) WithType(tenantType value_object.TenantType) *entity.Tenant {
	tenant := t.WithDefaults()
	tenant.Type = tenantType
	tenant.MaxUsers = tenantType.GetDefaultUserLimit()
	return tenant
}

// WithStatus crea un tenant con un estado específico
func (t TenantMother) WithStatus(status value_object.TenantStatus) *entity.Tenant {
	tenant := t.WithDefaults()
	tenant.Status = status
	return tenant
}

// WithPlan crea un tenant con un plan específico
func (t TenantMother) WithPlan(planID uuid.UUID) *entity.Tenant {
	tenant := t.WithDefaults()
	now := time.Now()
	tenant.PlanID = &planID
	tenant.SubscribedAt = &now
	return tenant
}

// WithDomain crea un tenant con un dominio personalizado
func (t TenantMother) WithDomain(domain string) *entity.Tenant {
	tenant := t.WithDefaults()
	tenant.Domain = domain
	return tenant
}

// WithUserLimits crea un tenant con límites de usuario específicos
func (t TenantMother) WithUserLimits(maxUsers, currentUsers int) *entity.Tenant {
	tenant := t.WithDefaults()
	tenant.MaxUsers = maxUsers
	tenant.UserCount = currentUsers
	return tenant
}

// WithExpiration crea un tenant con fecha de expiración
func (t TenantMother) WithExpiration(expiresAt time.Time) *entity.Tenant {
	tenant := t.WithDefaults()
	tenant.ExpiresAt = &expiresAt
	return tenant
}

// Startup crea un tenant tipo startup
func (t TenantMother) Startup() *entity.Tenant {
	tenant := t.WithDefaults()
	tenant.Type = value_object.TenantTypeStartup
	tenant.MaxUsers = value_object.TenantTypeStartup.GetDefaultUserLimit()
	return tenant
}

// Business crea un tenant tipo business
func (t TenantMother) Business() *entity.Tenant {
	tenant := t.WithDefaults()
	tenant.Type = value_object.TenantTypeBusiness
	tenant.MaxUsers = value_object.TenantTypeBusiness.GetDefaultUserLimit()
	return tenant
}

// Enterprise crea un tenant tipo enterprise
func (t TenantMother) Enterprise() *entity.Tenant {
	tenant := t.WithDefaults()
	tenant.Type = value_object.TenantTypeEnterprise
	tenant.MaxUsers = value_object.TenantTypeEnterprise.GetDefaultUserLimit()
	return tenant
}

// Suspended crea un tenant suspendido
func (t TenantMother) Suspended() *entity.Tenant {
	tenant := t.WithDefaults()
	tenant.Status = value_object.TenantStatusSuspended
	return tenant
}

// Deleted crea un tenant eliminado
func (t TenantMother) Deleted() *entity.Tenant {
	tenant := t.WithDefaults()
	tenant.Status = value_object.TenantStatusDeleted
	return tenant
}

// Expired crea un tenant expirado
func (t TenantMother) Expired() *entity.Tenant {
	tenant := t.WithDefaults()
	yesterday := time.Now().AddDate(0, 0, -1)
	tenant.ExpiresAt = &yesterday
	return tenant
}

// Complete crea un tenant con todos los parámetros especificados
func (TenantMother) Complete(id, ownerID uuid.UUID, name, slug, description, domain string,
	tenantType value_object.TenantType, status value_object.TenantStatus,
	maxUsers, userCount int, planID *uuid.UUID) *entity.Tenant {

	now := time.Now()
	var subscribedAt *time.Time
	if planID != nil {
		subscribedAt = &now
	}

	return &entity.Tenant{
		ID:           id,
		Name:         name,
		Slug:         slug,
		Description:  description,
		Type:         tenantType,
		Status:       status,
		PlanID:       planID,
		Domain:       domain,
		MaxUsers:     maxUsers,
		UserCount:    userCount,
		OwnerID:      ownerID,
		Settings:     make(map[string]interface{}),
		Features:     value_object.NewTenantFeatures(),
		SubscribedAt: subscribedAt,
		ExpiresAt:    nil,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// Create retorna una nueva instancia de TenantMother
func Create() TenantMother {
	return TenantMother{}
}
