package entity

import (
	"iam/src/tenant/domain/value_object"
	"time"

	"github.com/google/uuid"
)

type Tenant struct {
	ID           uuid.UUID
	Name         string
	Slug         string // Identificador único para URLs
	Description  string
	Type         value_object.TenantType
	Status       value_object.TenantStatus
	PlanID       *uuid.UUID                   // Relación con Plan
	Domain       string                       // Dominio personalizado (opcional)
	MaxUsers     int                          // Límite de usuarios
	UserCount    int                          // Usuarios actuales
	OwnerID      uuid.UUID                    // Usuario propietario
	Settings     map[string]interface{}       // Configuraciones personalizadas
	Features     *value_object.TenantFeatures // Feature flags del tenant
	SubscribedAt *time.Time                   // Fecha de suscripción
	ExpiresAt    *time.Time                   // Fecha de expiración
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func NewTenant(name, slug, description string, tenantType value_object.TenantType, ownerID uuid.UUID) *Tenant {
	return &Tenant{
		ID:          uuid.New(),
		Name:        name,
		Slug:        slug,
		Description: description,
		Type:        tenantType,
		Status:      value_object.TenantStatusActive,
		MaxUsers:    tenantType.GetDefaultUserLimit(),
		UserCount:   0,
		OwnerID:     ownerID,
		Settings:    make(map[string]interface{}),
		Features:    value_object.NewTenantFeatures(), // Inicializar con valores por defecto
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

func (t *Tenant) UpdateDetails(name, description string) {
	t.Name = name
	t.Description = description
	t.UpdatedAt = time.Now()
}

func (t *Tenant) ChangeStatus(status value_object.TenantStatus) {
	t.Status = status
	t.UpdatedAt = time.Now()
}

func (t *Tenant) Activate() {
	t.Status = value_object.TenantStatusActive
	t.UpdatedAt = time.Now()
}

func (t *Tenant) Suspend() {
	t.Status = value_object.TenantStatusSuspended
	t.UpdatedAt = time.Now()
}

func (t *Tenant) Delete() {
	t.Status = value_object.TenantStatusDeleted
	t.UpdatedAt = time.Now()
}

func (t *Tenant) SetPlan(planID uuid.UUID) {
	t.PlanID = &planID
	now := time.Now()
	t.SubscribedAt = &now
	t.UpdatedAt = now
}

func (t *Tenant) RemovePlan() {
	t.PlanID = nil
	t.SubscribedAt = nil
	t.ExpiresAt = nil
	t.UpdatedAt = time.Now()
}

func (t *Tenant) SetCustomDomain(domain string) {
	t.Domain = domain
	t.UpdatedAt = time.Now()
}

func (t *Tenant) UpdateUserLimits(maxUsers int) {
	t.MaxUsers = maxUsers
	t.UpdatedAt = time.Now()
}

func (t *Tenant) IncrementUserCount() error {
	if t.MaxUsers != -1 && t.UserCount >= t.MaxUsers {
		return &TenantError{Message: "user limit exceeded"}
	}
	t.UserCount++
	t.UpdatedAt = time.Now()
	return nil
}

func (t *Tenant) DecrementUserCount() {
	if t.UserCount > 0 {
		t.UserCount--
		t.UpdatedAt = time.Now()
	}
}

func (t *Tenant) SetExpiration(expiresAt time.Time) {
	t.ExpiresAt = &expiresAt
	t.UpdatedAt = time.Now()
}

func (t *Tenant) UpdateSetting(key string, value interface{}) {
	if t.Settings == nil {
		t.Settings = make(map[string]interface{})
	}
	t.Settings[key] = value
	t.UpdatedAt = time.Now()
}

func (t *Tenant) GetSetting(key string) (interface{}, bool) {
	if t.Settings == nil {
		return nil, false
	}
	value, exists := t.Settings[key]
	return value, exists
}

// Métodos para manejar feature flags
func (t *Tenant) UpdateFeatures(features *value_object.TenantFeatures) {
	t.Features = features
	t.UpdatedAt = time.Now()
}

func (t *Tenant) GetFeatures() *value_object.TenantFeatures {
	if t.Features == nil {
		t.Features = value_object.NewTenantFeatures()
	}
	return t.Features
}

func (t *Tenant) EnableFriendsFamily() {
	t.GetFeatures().UpdateFriendsFamily(true)
	t.UpdatedAt = time.Now()
}

func (t *Tenant) DisableFriendsFamily() {
	t.GetFeatures().UpdateFriendsFamily(false)
	t.UpdatedAt = time.Now()
}

func (t *Tenant) EnablePremiumAnalytics() {
	t.GetFeatures().UpdatePremiumAnalytics(true)
	t.UpdatedAt = time.Now()
}

func (t *Tenant) DisablePremiumAnalytics() {
	t.GetFeatures().UpdatePremiumAnalytics(false)
	t.UpdatedAt = time.Now()
}

func (t *Tenant) HasFriendsFamily() bool {
	return t.GetFeatures().HasFriendsFamily()
}

func (t *Tenant) HasPremiumAnalytics() bool {
	return t.GetFeatures().HasPremiumAnalytics()
}

// Métodos de consulta
func (t *Tenant) IsActive() bool {
	return t.Status.IsActive()
}

func (t *Tenant) CanAccess() bool {
	if !t.Status.CanAccess() {
		return false
	}

	// Verificar expiración
	if t.ExpiresAt != nil && time.Now().After(*t.ExpiresAt) {
		return false
	}

	return true
}

func (t *Tenant) IsExpired() bool {
	return t.ExpiresAt != nil && time.Now().After(*t.ExpiresAt)
}

func (t *Tenant) CanAddUser() bool {
	if !t.CanAccess() {
		return false
	}

	if t.MaxUsers == -1 {
		return true // Unlimited
	}

	return t.UserCount < t.MaxUsers
}

func (t *Tenant) HasPlan() bool {
	return t.PlanID != nil
}

func (t *Tenant) HasCustomDomain() bool {
	return t.Domain != ""
}

func (t *Tenant) CanBeModified() bool {
	return t.Status.CanBeModified()
}

// Error personalizado
type TenantError struct {
	Message string
}

func (e *TenantError) Error() string {
	return e.Message
}
