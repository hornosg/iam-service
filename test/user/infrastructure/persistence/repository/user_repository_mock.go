package repository

import (
	"context"
	"errors"
	"sync"

	"github.com/hornosg/go-shared/criteria"
	"iam/src/user/domain/entity"
	"iam/src/user/domain/value_object"

	"github.com/google/uuid"
)

// Errores mock
var (
	ErrMockFailedOp     = errors.New("operación fallida (simulada)")
	ErrMockNotFound     = errors.New("usuario no encontrado (simulado)")
	ErrMockDuplicated   = errors.New("usuario duplicado (simulado)")
	ErrMockInvalidEmail = errors.New("email inválido (simulado)")
)

// MockUserRepository implementa un repositorio en memoria para pruebas
type MockUserRepository struct {
	mu            sync.RWMutex
	users         map[uuid.UUID]*entity.User
	emailIndex    map[string]uuid.UUID // email -> userID para búsquedas rápidas
	shouldFail    bool
	failOnMethods map[string]bool
	callHistory   map[string]int
}

// NewMockUserRepository crea una nueva instancia del mock
func NewMockUserRepository() *MockUserRepository {
	return &MockUserRepository{
		users:         make(map[uuid.UUID]*entity.User),
		emailIndex:    make(map[string]uuid.UUID),
		failOnMethods: make(map[string]bool),
		callHistory:   make(map[string]int),
	}
}

// SetShouldFail configura si todas las operaciones deberían fallar
func (r *MockUserRepository) SetShouldFail(shouldFail bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shouldFail = shouldFail
}

// ShouldFailOn configura un método específico para que falle
func (r *MockUserRepository) ShouldFailOn(method string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failOnMethods[method] = true
}

// ResetFailures limpia todas las configuraciones de fallo
func (r *MockUserRepository) ResetFailures() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shouldFail = false
	r.failOnMethods = make(map[string]bool)
}

// ResetCallHistory reinicia los contadores de llamadas
func (r *MockUserRepository) ResetCallHistory() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callHistory = make(map[string]int)
}

// GetCallCount retorna el número de veces que se ha llamado a un método
func (r *MockUserRepository) GetCallCount(method string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.callHistory[method]
}

// SetupUsers inicializa el repositorio con usuarios predefinidos
func (r *MockUserRepository) SetupUsers(users []*entity.User) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.users = make(map[uuid.UUID]*entity.User)
	r.emailIndex = make(map[string]uuid.UUID)

	for _, user := range users {
		clonedUser := r.cloneUser(user)
		r.users[user.ID] = clonedUser
		r.emailIndex[user.Email.Value()] = user.ID
	}
}

// GetUsers retorna todos los usuarios almacenados
func (r *MockUserRepository) GetUsers() []*entity.User {
	r.mu.RLock()
	defer r.mu.RUnlock()

	users := make([]*entity.User, 0, len(r.users))
	for _, user := range r.users {
		users = append(users, r.cloneUser(user))
	}
	return users
}

// shouldMethodFail comprueba si un método debería fallar
func (r *MockUserRepository) shouldMethodFail(method string) bool {
	return r.shouldFail || r.failOnMethods[method]
}

// incrementCallCount incrementa el contador de llamadas para un método
func (r *MockUserRepository) incrementCallCount(method string) {
	r.callHistory[method] = r.callHistory[method] + 1
}

// cloneUser crea una copia profunda de un usuario
func (r *MockUserRepository) cloneUser(user *entity.User) *entity.User {
	return &entity.User{
		ID:           user.ID,
		Email:        user.Email,
		PasswordHash: user.PasswordHash,
		TenantID:     user.TenantID,
		RoleID:       user.RoleID,
		Status:       user.Status,
		Provider:     user.Provider,
		FederatedID:  user.FederatedID,
		CreatedAt:    user.CreatedAt,
		UpdatedAt:    user.UpdatedAt,
	}
}

// Create implementa la interfaz del repositorio
func (r *MockUserRepository) Create(ctx context.Context, user *entity.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.incrementCallCount("Create")

	if r.shouldMethodFail("Create") {
		return ErrMockFailedOp
	}

	// Verificar si ya existe un usuario con ese email
	if _, exists := r.emailIndex[user.Email.Value()]; exists {
		return ErrMockDuplicated
	}

	// Verificar si ya existe un usuario con ese ID
	if _, exists := r.users[user.ID]; exists {
		return ErrMockDuplicated
	}

	// Crear una copia para evitar referencia compartida
	clonedUser := r.cloneUser(user)
	r.users[user.ID] = clonedUser
	r.emailIndex[user.Email.Value()] = user.ID

	return nil
}

// GetByID implementa la interfaz del repositorio
func (r *MockUserRepository) GetByID(ctx context.Context, id uuid.UUID) (*entity.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetByID")

	if r.shouldMethodFail("GetByID") {
		return nil, ErrMockFailedOp
	}

	user, exists := r.users[id]
	if !exists {
		return nil, ErrMockNotFound
	}

	return r.cloneUser(user), nil
}

// GetByEmail implementa la interfaz del repositorio
func (r *MockUserRepository) GetByEmail(ctx context.Context, email string, tenantID *uuid.UUID) (*entity.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetByEmail")

	if r.shouldMethodFail("GetByEmail") {
		return nil, ErrMockFailedOp
	}

	userID, exists := r.emailIndex[email]
	if !exists {
		return nil, ErrMockNotFound
	}

	user := r.users[userID]

	// Si se especifica tenantID, verificar que coincida
	if tenantID != nil && user.TenantID != *tenantID {
		return nil, ErrMockNotFound
	}

	return r.cloneUser(user), nil
}

// Update implementa la interfaz del repositorio
func (r *MockUserRepository) Update(ctx context.Context, user *entity.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.incrementCallCount("Update")

	if r.shouldMethodFail("Update") {
		return ErrMockFailedOp
	}

	existingUser, exists := r.users[user.ID]
	if !exists {
		return ErrMockNotFound
	}

	// Si el email cambió, actualizar el índice
	if existingUser.Email.Value() != user.Email.Value() {
		// Verificar que el nuevo email no esté en uso
		if _, emailExists := r.emailIndex[user.Email.Value()]; emailExists {
			return ErrMockDuplicated
		}

		// Remover el email anterior del índice
		delete(r.emailIndex, existingUser.Email.Value())
		// Agregar el nuevo email al índice
		r.emailIndex[user.Email.Value()] = user.ID
	}

	// Actualizar el usuario
	r.users[user.ID] = r.cloneUser(user)

	return nil
}

// Delete implementa la interfaz del repositorio
func (r *MockUserRepository) Delete(ctx context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.incrementCallCount("Delete")

	if r.shouldMethodFail("Delete") {
		return ErrMockFailedOp
	}

	user, exists := r.users[id]
	if !exists {
		return ErrMockNotFound
	}

	// Soft delete: cambiar status a deleted
	user.Status = value_object.StatusDeleted
	r.users[id] = user

	return nil
}

// GetByTenant implementa la interfaz del repositorio
func (r *MockUserRepository) GetByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]*entity.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetByTenant")

	if r.shouldMethodFail("GetByTenant") {
		return nil, ErrMockFailedOp
	}

	var users []*entity.User
	count := 0

	for _, user := range r.users {
		if user.TenantID == tenantID && user.Status != value_object.StatusDeleted {
			if count >= offset {
				users = append(users, r.cloneUser(user))
				if len(users) >= limit {
					break
				}
			}
			count++
		}
	}

	return users, nil
}

// GetByStatus implementa la interfaz del repositorio
func (r *MockUserRepository) GetByStatus(ctx context.Context, status value_object.UserStatus, limit, offset int) ([]*entity.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetByStatus")

	if r.shouldMethodFail("GetByStatus") {
		return nil, ErrMockFailedOp
	}

	var users []*entity.User
	count := 0

	for _, user := range r.users {
		if user.Status == status {
			if count >= offset {
				users = append(users, r.cloneUser(user))
				if len(users) >= limit {
					break
				}
			}
			count++
		}
	}

	return users, nil
}

// GetByRole implementa la interfaz del repositorio
func (r *MockUserRepository) GetByRole(ctx context.Context, roleID uuid.UUID, limit, offset int) ([]*entity.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetByRole")

	if r.shouldMethodFail("GetByRole") {
		return nil, ErrMockFailedOp
	}

	var users []*entity.User
	count := 0

	for _, user := range r.users {
		if user.RoleID == roleID && user.Status != value_object.StatusDeleted {
			if count >= offset {
				users = append(users, r.cloneUser(user))
				if len(users) >= limit {
					break
				}
			}
			count++
		}
	}

	return users, nil
}

// ExistsByEmail implementa la interfaz del repositorio
func (r *MockUserRepository) ExistsByEmail(ctx context.Context, email string, tenantID *uuid.UUID) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("ExistsByEmail")

	if r.shouldMethodFail("ExistsByEmail") {
		return false, ErrMockFailedOp
	}

	userID, exists := r.emailIndex[email]
	if !exists {
		return false, nil
	}

	user := r.users[userID]

	// Verificar que no esté eliminado
	if user.Status == value_object.StatusDeleted {
		return false, nil
	}

	// Si se especifica tenantID, verificar que coincida
	if tenantID != nil && user.TenantID != *tenantID {
		return false, nil
	}

	return true, nil
}

// CountByTenant implementa la interfaz del repositorio
func (r *MockUserRepository) CountByTenant(ctx context.Context, tenantID uuid.UUID) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("CountByTenant")

	if r.shouldMethodFail("CountByTenant") {
		return 0, ErrMockFailedOp
	}

	count := 0
	for _, user := range r.users {
		if user.TenantID == tenantID && user.Status != value_object.StatusDeleted {
			count++
		}
	}

	return count, nil
}

// CountByStatus implementa la interfaz del repositorio
func (r *MockUserRepository) CountByStatus(ctx context.Context, status value_object.UserStatus) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("CountByStatus")

	if r.shouldMethodFail("CountByStatus") {
		return 0, ErrMockFailedOp
	}

	count := 0
	for _, user := range r.users {
		if user.Status == status {
			count++
		}
	}

	return count, nil
}

// SearchByCriteria implementa la interfaz del repositorio con criterios
func (r *MockUserRepository) SearchByCriteria(ctx context.Context, crit criteria.Criteria) ([]*entity.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("SearchByCriteria")

	if r.shouldMethodFail("SearchByCriteria") {
		return nil, ErrMockFailedOp
	}

	// Implementación básica de filtrado por criterios
	// En un mock real, podrías implementar filtros más sofisticados
	var users []*entity.User
	for _, user := range r.users {
		users = append(users, r.cloneUser(user))
	}

	return users, nil
}

// CountByCriteria implementa la interfaz del repositorio con criterios
func (r *MockUserRepository) CountByCriteria(ctx context.Context, crit criteria.Criteria) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("CountByCriteria")

	if r.shouldMethodFail("CountByCriteria") {
		return 0, ErrMockFailedOp
	}

	return len(r.users), nil
}
