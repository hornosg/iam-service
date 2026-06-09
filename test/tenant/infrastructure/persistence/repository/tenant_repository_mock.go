package repository

import (
	"context"
	"errors"
	"sync"
	"time"

	"iam/src/tenant/domain/entity"
	"iam/src/tenant/domain/value_object"

	"github.com/google/uuid"
)

// Errores mock
var (
	ErrMockFailedOp         = errors.New("operacion fallida (simulada)")
	ErrMockTenantNotFound   = errors.New("tenant no encontrado (simulado)")
	ErrMockTenantDuplicated = errors.New("tenant duplicado (simulado)")
)

// MockTenantRepository implementa un repositorio en memoria para pruebas de tenant
type MockTenantRepository struct {
	mu            sync.RWMutex
	tenants       map[uuid.UUID]*entity.Tenant
	slugIndex     map[string]uuid.UUID
	domainIndex   map[string]uuid.UUID
	shouldFail    bool
	failOnMethods map[string]bool
	callHistory   map[string]int
}

// NewMockTenantRepository crea una nueva instancia del mock
func NewMockTenantRepository() *MockTenantRepository {
	return &MockTenantRepository{
		tenants:       make(map[uuid.UUID]*entity.Tenant),
		slugIndex:     make(map[string]uuid.UUID),
		domainIndex:   make(map[string]uuid.UUID),
		failOnMethods: make(map[string]bool),
		callHistory:   make(map[string]int),
	}
}

// SetShouldFail configura si todas las operaciones deberian fallar
func (r *MockTenantRepository) SetShouldFail(shouldFail bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shouldFail = shouldFail
}

// ShouldFailOn configura un metodo especifico para que falle
func (r *MockTenantRepository) ShouldFailOn(method string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failOnMethods[method] = true
}

// ResetFailures limpia todas las configuraciones de fallo
func (r *MockTenantRepository) ResetFailures() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shouldFail = false
	r.failOnMethods = make(map[string]bool)
}

// ResetCallHistory reinicia los contadores de llamadas
func (r *MockTenantRepository) ResetCallHistory() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callHistory = make(map[string]int)
}

// GetCallCount retorna el numero de veces que se ha llamado a un metodo
func (r *MockTenantRepository) GetCallCount(method string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.callHistory[method]
}

// SetupTenants inicializa el repositorio con tenants predefinidos
func (r *MockTenantRepository) SetupTenants(tenants []*entity.Tenant) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tenants = make(map[uuid.UUID]*entity.Tenant)
	r.slugIndex = make(map[string]uuid.UUID)
	r.domainIndex = make(map[string]uuid.UUID)

	for _, tenant := range tenants {
		clonedTenant := r.cloneTenant(tenant)
		r.tenants[tenant.ID] = clonedTenant
		r.slugIndex[tenant.Slug] = tenant.ID
		if tenant.Domain != "" {
			r.domainIndex[tenant.Domain] = tenant.ID
		}
	}
}

// GetTenants retorna todos los tenants almacenados
func (r *MockTenantRepository) GetTenants() []*entity.Tenant {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tenants := make([]*entity.Tenant, 0, len(r.tenants))
	for _, tenant := range r.tenants {
		tenants = append(tenants, r.cloneTenant(tenant))
	}
	return tenants
}

// shouldMethodFail comprueba si un metodo deberia fallar
func (r *MockTenantRepository) shouldMethodFail(method string) bool {
	return r.shouldFail || r.failOnMethods[method]
}

// incrementCallCount incrementa el contador de llamadas para un metodo
func (r *MockTenantRepository) incrementCallCount(method string) {
	r.callHistory[method] = r.callHistory[method] + 1
}

// cloneTenant crea una copia profunda de un tenant
func (r *MockTenantRepository) cloneTenant(tenant *entity.Tenant) *entity.Tenant {
	settings := make(map[string]interface{})
	for k, v := range tenant.Settings {
		settings[k] = v
	}

	var planID *uuid.UUID
	if tenant.PlanID != nil {
		pid := *tenant.PlanID
		planID = &pid
	}

	var subscribedAt *time.Time
	if tenant.SubscribedAt != nil {
		sa := *tenant.SubscribedAt
		subscribedAt = &sa
	}

	var expiresAt *time.Time
	if tenant.ExpiresAt != nil {
		ea := *tenant.ExpiresAt
		expiresAt = &ea
	}

	var features *value_object.TenantFeatures
	if tenant.Features != nil {
		features = value_object.NewTenantFeaturesWithValues(
			tenant.Features.FriendsFamily,
			tenant.Features.PremiumAnalytics,
		)
	}

	return &entity.Tenant{
		ID:           tenant.ID,
		Name:         tenant.Name,
		Slug:         tenant.Slug,
		Description:  tenant.Description,
		Type:         tenant.Type,
		Status:       tenant.Status,
		PlanID:       planID,
		Domain:       tenant.Domain,
		MaxUsers:     tenant.MaxUsers,
		UserCount:    tenant.UserCount,
		OwnerID:      tenant.OwnerID,
		Settings:     settings,
		Features:     features,
		SubscribedAt: subscribedAt,
		ExpiresAt:    expiresAt,
		CreatedAt:    tenant.CreatedAt,
		UpdatedAt:    tenant.UpdatedAt,
	}
}

// Create implementa la interfaz del repositorio
func (r *MockTenantRepository) Create(ctx context.Context, tenant *entity.Tenant) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.incrementCallCount("Create")

	if r.shouldMethodFail("Create") {
		return ErrMockFailedOp
	}

	if _, exists := r.slugIndex[tenant.Slug]; exists {
		return ErrMockTenantDuplicated
	}

	if _, exists := r.tenants[tenant.ID]; exists {
		return ErrMockTenantDuplicated
	}

	clonedTenant := r.cloneTenant(tenant)
	r.tenants[tenant.ID] = clonedTenant
	r.slugIndex[tenant.Slug] = tenant.ID
	if tenant.Domain != "" {
		r.domainIndex[tenant.Domain] = tenant.ID
	}

	return nil
}

// GetByID implementa la interfaz del repositorio
func (r *MockTenantRepository) GetByID(ctx context.Context, id uuid.UUID) (*entity.Tenant, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetByID")

	if r.shouldMethodFail("GetByID") {
		return nil, ErrMockFailedOp
	}

	tenant, exists := r.tenants[id]
	if !exists {
		return nil, ErrMockTenantNotFound
	}

	return r.cloneTenant(tenant), nil
}

// GetBySlug implementa la interfaz del repositorio
func (r *MockTenantRepository) GetBySlug(ctx context.Context, slug string) (*entity.Tenant, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetBySlug")

	if r.shouldMethodFail("GetBySlug") {
		return nil, ErrMockFailedOp
	}

	tenantID, exists := r.slugIndex[slug]
	if !exists {
		return nil, ErrMockTenantNotFound
	}

	tenant := r.tenants[tenantID]
	return r.cloneTenant(tenant), nil
}

// GetByDomain implementa la interfaz del repositorio
func (r *MockTenantRepository) GetByDomain(ctx context.Context, domain string) (*entity.Tenant, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetByDomain")

	if r.shouldMethodFail("GetByDomain") {
		return nil, ErrMockFailedOp
	}

	tenantID, exists := r.domainIndex[domain]
	if !exists {
		return nil, ErrMockTenantNotFound
	}

	tenant := r.tenants[tenantID]
	return r.cloneTenant(tenant), nil
}

// Update implementa la interfaz del repositorio
func (r *MockTenantRepository) Update(ctx context.Context, tenant *entity.Tenant) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.incrementCallCount("Update")

	if r.shouldMethodFail("Update") {
		return ErrMockFailedOp
	}

	_, exists := r.tenants[tenant.ID]
	if !exists {
		return ErrMockTenantNotFound
	}

	r.tenants[tenant.ID] = r.cloneTenant(tenant)

	return nil
}

// Delete implementa la interfaz del repositorio
func (r *MockTenantRepository) Delete(ctx context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.incrementCallCount("Delete")

	if r.shouldMethodFail("Delete") {
		return ErrMockFailedOp
	}

	tenant, exists := r.tenants[id]
	if !exists {
		return ErrMockTenantNotFound
	}

	tenant.Status = value_object.TenantStatusDeleted
	r.tenants[id] = tenant

	return nil
}

// GetByOwner implementa la interfaz del repositorio
func (r *MockTenantRepository) GetByOwner(ctx context.Context, ownerID uuid.UUID) ([]*entity.Tenant, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetByOwner")

	if r.shouldMethodFail("GetByOwner") {
		return nil, ErrMockFailedOp
	}

	var tenants []*entity.Tenant
	for _, tenant := range r.tenants {
		if tenant.OwnerID == ownerID {
			tenants = append(tenants, r.cloneTenant(tenant))
		}
	}

	return tenants, nil
}

// GetByStatus implementa la interfaz del repositorio
func (r *MockTenantRepository) GetByStatus(ctx context.Context, status value_object.TenantStatus, limit, offset int) ([]*entity.Tenant, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetByStatus")

	if r.shouldMethodFail("GetByStatus") {
		return nil, ErrMockFailedOp
	}

	var tenants []*entity.Tenant
	count := 0

	for _, tenant := range r.tenants {
		if tenant.Status == status {
			if count >= offset {
				tenants = append(tenants, r.cloneTenant(tenant))
				if limit > 0 && len(tenants) >= limit {
					break
				}
			}
			count++
		}
	}

	return tenants, nil
}

// GetByType implementa la interfaz del repositorio
func (r *MockTenantRepository) GetByType(ctx context.Context, tenantType value_object.TenantType, limit, offset int) ([]*entity.Tenant, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetByType")

	if r.shouldMethodFail("GetByType") {
		return nil, ErrMockFailedOp
	}

	var tenants []*entity.Tenant
	count := 0

	for _, tenant := range r.tenants {
		if tenant.Type == tenantType {
			if count >= offset {
				tenants = append(tenants, r.cloneTenant(tenant))
				if limit > 0 && len(tenants) >= limit {
					break
				}
			}
			count++
		}
	}

	return tenants, nil
}

// GetByPlan implementa la interfaz del repositorio
func (r *MockTenantRepository) GetByPlan(ctx context.Context, planID uuid.UUID, limit, offset int) ([]*entity.Tenant, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetByPlan")

	if r.shouldMethodFail("GetByPlan") {
		return nil, ErrMockFailedOp
	}

	var tenants []*entity.Tenant
	count := 0

	for _, tenant := range r.tenants {
		if tenant.PlanID != nil && *tenant.PlanID == planID {
			if count >= offset {
				tenants = append(tenants, r.cloneTenant(tenant))
				if limit > 0 && len(tenants) >= limit {
					break
				}
			}
			count++
		}
	}

	return tenants, nil
}

// GetActive implementa la interfaz del repositorio
func (r *MockTenantRepository) GetActive(ctx context.Context, limit, offset int) ([]*entity.Tenant, error) {
	return r.GetByStatus(ctx, value_object.TenantStatusActive, limit, offset)
}

// GetExpiring implementa la interfaz del repositorio
func (r *MockTenantRepository) GetExpiring(ctx context.Context, days int) ([]*entity.Tenant, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetExpiring")

	if r.shouldMethodFail("GetExpiring") {
		return nil, ErrMockFailedOp
	}

	deadline := time.Now().AddDate(0, 0, days)
	var tenants []*entity.Tenant

	for _, tenant := range r.tenants {
		if tenant.ExpiresAt != nil && tenant.ExpiresAt.Before(deadline) {
			tenants = append(tenants, r.cloneTenant(tenant))
		}
	}

	return tenants, nil
}

// List implementa la interfaz del repositorio
func (r *MockTenantRepository) List(ctx context.Context, limit, offset int) ([]*entity.Tenant, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("List")

	if r.shouldMethodFail("List") {
		return nil, ErrMockFailedOp
	}

	var tenants []*entity.Tenant
	count := 0

	for _, tenant := range r.tenants {
		if count >= offset {
			tenants = append(tenants, r.cloneTenant(tenant))
			if limit > 0 && len(tenants) >= limit {
				break
			}
		}
		count++
	}

	return tenants, nil
}

// ExistsBySlug implementa la interfaz del repositorio
func (r *MockTenantRepository) ExistsBySlug(ctx context.Context, slug string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("ExistsBySlug")

	if r.shouldMethodFail("ExistsBySlug") {
		return false, ErrMockFailedOp
	}

	_, exists := r.slugIndex[slug]
	return exists, nil
}

// ExistsByDomain implementa la interfaz del repositorio
func (r *MockTenantRepository) ExistsByDomain(ctx context.Context, domain string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("ExistsByDomain")

	if r.shouldMethodFail("ExistsByDomain") {
		return false, ErrMockFailedOp
	}

	_, exists := r.domainIndex[domain]
	return exists, nil
}

// Count implementa la interfaz del repositorio
func (r *MockTenantRepository) Count(ctx context.Context) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("Count")

	if r.shouldMethodFail("Count") {
		return 0, ErrMockFailedOp
	}

	return len(r.tenants), nil
}

// CountByStatus implementa la interfaz del repositorio
func (r *MockTenantRepository) CountByStatus(ctx context.Context, status value_object.TenantStatus) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("CountByStatus")

	if r.shouldMethodFail("CountByStatus") {
		return 0, ErrMockFailedOp
	}

	count := 0
	for _, tenant := range r.tenants {
		if tenant.Status == status {
			count++
		}
	}

	return count, nil
}

// CountByOwner implementa la interfaz del repositorio
func (r *MockTenantRepository) CountByOwner(ctx context.Context, ownerID uuid.UUID) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("CountByOwner")

	if r.shouldMethodFail("CountByOwner") {
		return 0, ErrMockFailedOp
	}

	count := 0
	for _, tenant := range r.tenants {
		if tenant.OwnerID == ownerID {
			count++
		}
	}

	return count, nil
}

// CountByPlan implementa la interfaz del repositorio
func (r *MockTenantRepository) CountByPlan(ctx context.Context, planID uuid.UUID) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("CountByPlan")

	if r.shouldMethodFail("CountByPlan") {
		return 0, ErrMockFailedOp
	}

	count := 0
	for _, tenant := range r.tenants {
		if tenant.PlanID != nil && *tenant.PlanID == planID {
			count++
		}
	}

	return count, nil
}
