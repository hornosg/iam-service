package repository

import (
	"context"
	"errors"
	"sync"

	"iam/src/role/domain/entity"
	"iam/src/role/domain/value_object"

	"github.com/google/uuid"
)

// Errores mock
var (
	ErrMockFailedOp       = errors.New("operacion fallida (simulada)")
	ErrMockRoleNotFound   = errors.New("rol no encontrado (simulado)")
	ErrMockRoleDuplicated = errors.New("rol duplicado (simulado)")
)

// MockRoleRepository implementa un repositorio en memoria para pruebas de role
type MockRoleRepository struct {
	mu            sync.RWMutex
	roles         map[uuid.UUID]*entity.Role
	nameIndex     map[string]uuid.UUID // "name:tenantID" -> roleID
	shouldFail    bool
	failOnMethods map[string]bool
	callHistory   map[string]int
}

// NewMockRoleRepository crea una nueva instancia del mock
func NewMockRoleRepository() *MockRoleRepository {
	return &MockRoleRepository{
		roles:         make(map[uuid.UUID]*entity.Role),
		nameIndex:     make(map[string]uuid.UUID),
		failOnMethods: make(map[string]bool),
		callHistory:   make(map[string]int),
	}
}

// SetShouldFail configura si todas las operaciones deberian fallar
func (r *MockRoleRepository) SetShouldFail(shouldFail bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shouldFail = shouldFail
}

// ShouldFailOn configura un metodo especifico para que falle
func (r *MockRoleRepository) ShouldFailOn(method string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failOnMethods[method] = true
}

// ResetFailures limpia todas las configuraciones de fallo
func (r *MockRoleRepository) ResetFailures() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shouldFail = false
	r.failOnMethods = make(map[string]bool)
}

// ResetCallHistory reinicia los contadores de llamadas
func (r *MockRoleRepository) ResetCallHistory() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callHistory = make(map[string]int)
}

// GetCallCount retorna el numero de veces que se ha llamado a un metodo
func (r *MockRoleRepository) GetCallCount(method string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.callHistory[method]
}

// SetupRoles inicializa el repositorio con roles predefinidos
func (r *MockRoleRepository) SetupRoles(roles []*entity.Role) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.roles = make(map[uuid.UUID]*entity.Role)
	r.nameIndex = make(map[string]uuid.UUID)

	for _, role := range roles {
		clonedRole := r.cloneRole(role)
		r.roles[role.ID] = clonedRole
		r.nameIndex[r.nameKey(role.Name, role.TenantID)] = role.ID
	}
}

// GetRoles retorna todos los roles almacenados
func (r *MockRoleRepository) GetRoles() []*entity.Role {
	r.mu.RLock()
	defer r.mu.RUnlock()

	roles := make([]*entity.Role, 0, len(r.roles))
	for _, role := range r.roles {
		roles = append(roles, r.cloneRole(role))
	}
	return roles
}

// shouldMethodFail comprueba si un metodo deberia fallar
func (r *MockRoleRepository) shouldMethodFail(method string) bool {
	return r.shouldFail || r.failOnMethods[method]
}

// incrementCallCount incrementa el contador de llamadas para un metodo
func (r *MockRoleRepository) incrementCallCount(method string) {
	r.callHistory[method] = r.callHistory[method] + 1
}

// nameKey genera una clave unica para el indice de nombres
func (r *MockRoleRepository) nameKey(name string, tenantID *uuid.UUID) string {
	if tenantID == nil {
		return name + ":system"
	}
	return name + ":" + tenantID.String()
}

// cloneRole crea una copia profunda de un role
func (r *MockRoleRepository) cloneRole(role *entity.Role) *entity.Role {
	permissions := make([]string, len(role.Permissions))
	copy(permissions, role.Permissions)

	var tenantID *uuid.UUID
	if role.TenantID != nil {
		tid := *role.TenantID
		tenantID = &tid
	}

	return &entity.Role{
		ID:          role.ID,
		Name:        role.Name,
		Description: role.Description,
		Type:        role.Type,
		TenantID:    tenantID,
		Permissions: permissions,
		IsActive:    role.IsActive,
		CreatedAt:   role.CreatedAt,
		UpdatedAt:   role.UpdatedAt,
	}
}

// Create implementa la interfaz del repositorio
func (r *MockRoleRepository) Create(ctx context.Context, role *entity.Role) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.incrementCallCount("Create")

	if r.shouldMethodFail("Create") {
		return ErrMockFailedOp
	}

	key := r.nameKey(role.Name, role.TenantID)
	if _, exists := r.nameIndex[key]; exists {
		return ErrMockRoleDuplicated
	}

	if _, exists := r.roles[role.ID]; exists {
		return ErrMockRoleDuplicated
	}

	clonedRole := r.cloneRole(role)
	r.roles[role.ID] = clonedRole
	r.nameIndex[key] = role.ID

	return nil
}

// GetByID implementa la interfaz del repositorio
func (r *MockRoleRepository) GetByID(ctx context.Context, id uuid.UUID) (*entity.Role, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetByID")

	if r.shouldMethodFail("GetByID") {
		return nil, ErrMockFailedOp
	}

	role, exists := r.roles[id]
	if !exists {
		return nil, ErrMockRoleNotFound
	}

	return r.cloneRole(role), nil
}

// GetByName implementa la interfaz del repositorio
func (r *MockRoleRepository) GetByName(ctx context.Context, name string, tenantID *uuid.UUID) (*entity.Role, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetByName")

	if r.shouldMethodFail("GetByName") {
		return nil, ErrMockFailedOp
	}

	key := r.nameKey(name, tenantID)
	roleID, exists := r.nameIndex[key]
	if !exists {
		return nil, ErrMockRoleNotFound
	}

	role := r.roles[roleID]
	return r.cloneRole(role), nil
}

// Update implementa la interfaz del repositorio
func (r *MockRoleRepository) Update(ctx context.Context, role *entity.Role) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.incrementCallCount("Update")

	if r.shouldMethodFail("Update") {
		return ErrMockFailedOp
	}

	existingRole, exists := r.roles[role.ID]
	if !exists {
		return ErrMockRoleNotFound
	}

	// Si el nombre cambio, actualizar el indice
	oldKey := r.nameKey(existingRole.Name, existingRole.TenantID)
	newKey := r.nameKey(role.Name, role.TenantID)
	if oldKey != newKey {
		if _, nameExists := r.nameIndex[newKey]; nameExists {
			return ErrMockRoleDuplicated
		}
		delete(r.nameIndex, oldKey)
		r.nameIndex[newKey] = role.ID
	}

	r.roles[role.ID] = r.cloneRole(role)

	return nil
}

// Delete implementa la interfaz del repositorio
func (r *MockRoleRepository) Delete(ctx context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.incrementCallCount("Delete")

	if r.shouldMethodFail("Delete") {
		return ErrMockFailedOp
	}

	role, exists := r.roles[id]
	if !exists {
		return ErrMockRoleNotFound
	}

	role.IsActive = false
	r.roles[id] = role

	return nil
}

// GetByType implementa la interfaz del repositorio
func (r *MockRoleRepository) GetByType(ctx context.Context, roleType value_object.RoleType) ([]*entity.Role, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetByType")

	if r.shouldMethodFail("GetByType") {
		return nil, ErrMockFailedOp
	}

	var roles []*entity.Role
	for _, role := range r.roles {
		if role.Type == roleType {
			roles = append(roles, r.cloneRole(role))
		}
	}

	return roles, nil
}

// GetByTenant implementa la interfaz del repositorio
func (r *MockRoleRepository) GetByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]*entity.Role, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetByTenant")

	if r.shouldMethodFail("GetByTenant") {
		return nil, ErrMockFailedOp
	}

	var roles []*entity.Role
	count := 0

	for _, role := range r.roles {
		if role.TenantID != nil && *role.TenantID == tenantID {
			if count >= offset {
				roles = append(roles, r.cloneRole(role))
				if limit > 0 && len(roles) >= limit {
					break
				}
			}
			count++
		}
	}

	return roles, nil
}

// GetSystemRoles implementa la interfaz del repositorio
func (r *MockRoleRepository) GetSystemRoles(ctx context.Context) ([]*entity.Role, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetSystemRoles")

	if r.shouldMethodFail("GetSystemRoles") {
		return nil, ErrMockFailedOp
	}

	var roles []*entity.Role
	for _, role := range r.roles {
		if role.TenantID == nil {
			roles = append(roles, r.cloneRole(role))
		}
	}

	return roles, nil
}

// GetActiveRoles implementa la interfaz del repositorio
func (r *MockRoleRepository) GetActiveRoles(ctx context.Context, tenantID *uuid.UUID, limit, offset int) ([]*entity.Role, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetActiveRoles")

	if r.shouldMethodFail("GetActiveRoles") {
		return nil, ErrMockFailedOp
	}

	var roles []*entity.Role
	count := 0

	for _, role := range r.roles {
		if !role.IsActive {
			continue
		}
		matchesTenant := tenantID == nil ||
			(role.TenantID != nil && *role.TenantID == *tenantID) ||
			role.TenantID == nil
		if matchesTenant {
			if count >= offset {
				roles = append(roles, r.cloneRole(role))
				if limit > 0 && len(roles) >= limit {
					break
				}
			}
			count++
		}
	}

	return roles, nil
}

// List implementa la interfaz del repositorio
func (r *MockRoleRepository) List(ctx context.Context, limit, offset int) ([]*entity.Role, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("List")

	if r.shouldMethodFail("List") {
		return nil, ErrMockFailedOp
	}

	var roles []*entity.Role
	count := 0

	for _, role := range r.roles {
		if count >= offset {
			roles = append(roles, r.cloneRole(role))
			if limit > 0 && len(roles) >= limit {
				break
			}
		}
		count++
	}

	return roles, nil
}

// ExistsByName implementa la interfaz del repositorio
func (r *MockRoleRepository) ExistsByName(ctx context.Context, name string, tenantID *uuid.UUID) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("ExistsByName")

	if r.shouldMethodFail("ExistsByName") {
		return false, ErrMockFailedOp
	}

	key := r.nameKey(name, tenantID)
	_, exists := r.nameIndex[key]
	return exists, nil
}

// Count implementa la interfaz del repositorio
func (r *MockRoleRepository) Count(ctx context.Context) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("Count")

	if r.shouldMethodFail("Count") {
		return 0, ErrMockFailedOp
	}

	return len(r.roles), nil
}

// CountByTenant implementa la interfaz del repositorio
func (r *MockRoleRepository) CountByTenant(ctx context.Context, tenantID uuid.UUID) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("CountByTenant")

	if r.shouldMethodFail("CountByTenant") {
		return 0, ErrMockFailedOp
	}

	count := 0
	for _, role := range r.roles {
		if role.TenantID != nil && *role.TenantID == tenantID {
			count++
		}
	}

	return count, nil
}

// CountByType implementa la interfaz del repositorio
func (r *MockRoleRepository) CountByType(ctx context.Context, roleType value_object.RoleType) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("CountByType")

	if r.shouldMethodFail("CountByType") {
		return 0, ErrMockFailedOp
	}

	count := 0
	for _, role := range r.roles {
		if role.Type == roleType {
			count++
		}
	}

	return count, nil
}
