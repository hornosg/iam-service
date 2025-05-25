package repository

import (
	"context"
	"errors"
	"sync"

	"iam/src/plan/domain/entity"
	"iam/src/plan/domain/value_object"

	"github.com/google/uuid"
)

// Errores mock
var (
	ErrMockFailedOp       = errors.New("operación fallida (simulada)")
	ErrMockPlanNotFound   = errors.New("plan no encontrado (simulado)")
	ErrMockPlanDuplicated = errors.New("plan duplicado (simulado)")
)

// MockPlanRepository implementa un repositorio en memoria para pruebas de plan
type MockPlanRepository struct {
	mu            sync.RWMutex
	plans         map[uuid.UUID]*entity.Plan
	nameIndex     map[string]uuid.UUID // name -> planID para búsquedas rápidas
	shouldFail    bool
	failOnMethods map[string]bool
	callHistory   map[string]int
}

// NewMockPlanRepository crea una nueva instancia del mock
func NewMockPlanRepository() *MockPlanRepository {
	return &MockPlanRepository{
		plans:         make(map[uuid.UUID]*entity.Plan),
		nameIndex:     make(map[string]uuid.UUID),
		failOnMethods: make(map[string]bool),
		callHistory:   make(map[string]int),
	}
}

// SetShouldFail configura si todas las operaciones deberían fallar
func (r *MockPlanRepository) SetShouldFail(shouldFail bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shouldFail = shouldFail
}

// ShouldFailOn configura un método específico para que falle
func (r *MockPlanRepository) ShouldFailOn(method string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failOnMethods[method] = true
}

// ResetFailures limpia todas las configuraciones de fallo
func (r *MockPlanRepository) ResetFailures() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shouldFail = false
	r.failOnMethods = make(map[string]bool)
}

// ResetCallHistory reinicia los contadores de llamadas
func (r *MockPlanRepository) ResetCallHistory() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callHistory = make(map[string]int)
}

// GetCallCount retorna el número de veces que se ha llamado a un método
func (r *MockPlanRepository) GetCallCount(method string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.callHistory[method]
}

// SetupPlans inicializa el repositorio con planes predefinidos
func (r *MockPlanRepository) SetupPlans(plans []*entity.Plan) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plans = make(map[uuid.UUID]*entity.Plan)
	r.nameIndex = make(map[string]uuid.UUID)

	for _, plan := range plans {
		clonedPlan := r.clonePlan(plan)
		r.plans[plan.ID] = clonedPlan
		r.nameIndex[plan.Name] = plan.ID
	}
}

// GetPlans retorna todos los planes almacenados
func (r *MockPlanRepository) GetPlans() []*entity.Plan {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plans := make([]*entity.Plan, 0, len(r.plans))
	for _, plan := range r.plans {
		plans = append(plans, r.clonePlan(plan))
	}
	return plans
}

// shouldMethodFail comprueba si un método debería fallar
func (r *MockPlanRepository) shouldMethodFail(method string) bool {
	return r.shouldFail || r.failOnMethods[method]
}

// incrementCallCount incrementa el contador de llamadas para un método
func (r *MockPlanRepository) incrementCallCount(method string) {
	r.callHistory[method] = r.callHistory[method] + 1
}

// clonePlan crea una copia profunda de un plan
func (r *MockPlanRepository) clonePlan(plan *entity.Plan) *entity.Plan {
	features := make([]string, len(plan.Features))
	copy(features, plan.Features)

	return &entity.Plan{
		ID:          plan.ID,
		Name:        plan.Name,
		Description: plan.Description,
		Type:        plan.Type,
		Status:      plan.Status,
		MaxUsers:    plan.MaxUsers,
		PriceMonth:  plan.PriceMonth,
		PriceYear:   plan.PriceYear,
		Features:    features,
		CreatedAt:   plan.CreatedAt,
		UpdatedAt:   plan.UpdatedAt,
	}
}

// Create implementa la interfaz del repositorio
func (r *MockPlanRepository) Create(ctx context.Context, plan *entity.Plan) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.incrementCallCount("Create")

	if r.shouldMethodFail("Create") {
		return ErrMockFailedOp
	}

	// Verificar si ya existe un plan con ese nombre
	if _, exists := r.nameIndex[plan.Name]; exists {
		return ErrMockPlanDuplicated
	}

	// Verificar si ya existe un plan con ese ID
	if _, exists := r.plans[plan.ID]; exists {
		return ErrMockPlanDuplicated
	}

	// Crear una copia para evitar referencia compartida
	clonedPlan := r.clonePlan(plan)
	r.plans[plan.ID] = clonedPlan
	r.nameIndex[plan.Name] = plan.ID

	return nil
}

// GetByID implementa la interfaz del repositorio
func (r *MockPlanRepository) GetByID(ctx context.Context, id uuid.UUID) (*entity.Plan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetByID")

	if r.shouldMethodFail("GetByID") {
		return nil, ErrMockFailedOp
	}

	plan, exists := r.plans[id]
	if !exists {
		return nil, ErrMockPlanNotFound
	}

	return r.clonePlan(plan), nil
}

// GetByName implementa la interfaz del repositorio
func (r *MockPlanRepository) GetByName(ctx context.Context, name string) (*entity.Plan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetByName")

	if r.shouldMethodFail("GetByName") {
		return nil, ErrMockFailedOp
	}

	planID, exists := r.nameIndex[name]
	if !exists {
		return nil, ErrMockPlanNotFound
	}

	plan := r.plans[planID]
	return r.clonePlan(plan), nil
}

// Update implementa la interfaz del repositorio
func (r *MockPlanRepository) Update(ctx context.Context, plan *entity.Plan) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.incrementCallCount("Update")

	if r.shouldMethodFail("Update") {
		return ErrMockFailedOp
	}

	existingPlan, exists := r.plans[plan.ID]
	if !exists {
		return ErrMockPlanNotFound
	}

	// Si el nombre cambió, actualizar el índice
	if existingPlan.Name != plan.Name {
		// Verificar que el nuevo nombre no esté en uso
		if _, nameExists := r.nameIndex[plan.Name]; nameExists {
			return ErrMockPlanDuplicated
		}

		// Remover el nombre anterior del índice
		delete(r.nameIndex, existingPlan.Name)
		// Agregar el nuevo nombre al índice
		r.nameIndex[plan.Name] = plan.ID
	}

	// Actualizar el plan
	r.plans[plan.ID] = r.clonePlan(plan)

	return nil
}

// Delete implementa la interfaz del repositorio (soft delete)
func (r *MockPlanRepository) Delete(ctx context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.incrementCallCount("Delete")

	if r.shouldMethodFail("Delete") {
		return ErrMockFailedOp
	}

	plan, exists := r.plans[id]
	if !exists {
		return ErrMockPlanNotFound
	}

	// Soft delete: cambiar status a deprecated
	plan.Status = value_object.PlanStatusDeprecated
	r.plans[id] = plan

	return nil
}

// GetByType implementa la interfaz del repositorio
func (r *MockPlanRepository) GetByType(ctx context.Context, planType value_object.PlanType) ([]*entity.Plan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetByType")

	if r.shouldMethodFail("GetByType") {
		return nil, ErrMockFailedOp
	}

	var plans []*entity.Plan
	for _, plan := range r.plans {
		if plan.Type == planType {
			plans = append(plans, r.clonePlan(plan))
		}
	}

	return plans, nil
}

// GetByStatus implementa la interfaz del repositorio
func (r *MockPlanRepository) GetByStatus(ctx context.Context, status value_object.PlanStatus) ([]*entity.Plan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetByStatus")

	if r.shouldMethodFail("GetByStatus") {
		return nil, ErrMockFailedOp
	}

	var plans []*entity.Plan
	for _, plan := range r.plans {
		if plan.Status == status {
			plans = append(plans, r.clonePlan(plan))
		}
	}

	return plans, nil
}

// GetActive implementa la interfaz del repositorio
func (r *MockPlanRepository) GetActive(ctx context.Context) ([]*entity.Plan, error) {
	return r.GetByStatus(ctx, value_object.PlanStatusActive)
}

// List implementa la interfaz del repositorio con paginación
func (r *MockPlanRepository) List(ctx context.Context, limit, offset int) ([]*entity.Plan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("List")

	if r.shouldMethodFail("List") {
		return nil, ErrMockFailedOp
	}

	var plans []*entity.Plan
	count := 0

	for _, plan := range r.plans {
		if count >= offset {
			plans = append(plans, r.clonePlan(plan))
			if len(plans) >= limit {
				break
			}
		}
		count++
	}

	return plans, nil
}

// ExistsByName implementa la interfaz del repositorio
func (r *MockPlanRepository) ExistsByName(ctx context.Context, name string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("ExistsByName")

	if r.shouldMethodFail("ExistsByName") {
		return false, ErrMockFailedOp
	}

	_, exists := r.nameIndex[name]
	return exists, nil
}

// Count implementa la interfaz del repositorio
func (r *MockPlanRepository) Count(ctx context.Context) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("Count")

	if r.shouldMethodFail("Count") {
		return 0, ErrMockFailedOp
	}

	return len(r.plans), nil
}

// CountByStatus implementa la interfaz del repositorio
func (r *MockPlanRepository) CountByStatus(ctx context.Context, status value_object.PlanStatus) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("CountByStatus")

	if r.shouldMethodFail("CountByStatus") {
		return 0, ErrMockFailedOp
	}

	count := 0
	for _, plan := range r.plans {
		if plan.Status == status {
			count++
		}
	}

	return count, nil
}
