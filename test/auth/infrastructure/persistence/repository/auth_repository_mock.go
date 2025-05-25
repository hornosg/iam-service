package repository

import (
	"context"
	"errors"
	"sync"

	"iam/src/auth/domain/entity"
	"iam/src/auth/domain/port"
	"iam/src/auth/domain/value_object"

	"github.com/google/uuid"
)

// Errores mock
var (
	ErrMockFailedOp        = errors.New("operación fallida (simulada)")
	ErrMockTokenNotFound   = errors.New("refresh token no encontrado (simulado)")
	ErrMockUserNotFound    = errors.New("usuario no encontrado (simulado)")
	ErrMockDuplicatedToken = errors.New("token duplicado (simulado)")
	ErrMockInvalidProvider = errors.New("proveedor inválido (simulado)")
)

// MockAuthRepository implementa un repositorio en memoria para pruebas de auth
type MockAuthRepository struct {
	mu             sync.RWMutex
	refreshTokens  map[string]*entity.RefreshToken
	tokensByUser   map[uuid.UUID][]*entity.RefreshToken
	federatedUsers map[string]port.UserData // key: provider:federatedID
	shouldFail     bool
	failOnMethods  map[string]bool
	callHistory    map[string]int
}

// NewMockAuthRepository crea una nueva instancia del mock
func NewMockAuthRepository() *MockAuthRepository {
	return &MockAuthRepository{
		refreshTokens:  make(map[string]*entity.RefreshToken),
		tokensByUser:   make(map[uuid.UUID][]*entity.RefreshToken),
		federatedUsers: make(map[string]port.UserData),
		failOnMethods:  make(map[string]bool),
		callHistory:    make(map[string]int),
	}
}

// SetShouldFail configura si todas las operaciones deberían fallar
func (r *MockAuthRepository) SetShouldFail(shouldFail bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shouldFail = shouldFail
}

// ShouldFailOn configura un método específico para que falle
func (r *MockAuthRepository) ShouldFailOn(method string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failOnMethods[method] = true
}

// ResetFailures limpia todas las configuraciones de fallo
func (r *MockAuthRepository) ResetFailures() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shouldFail = false
	r.failOnMethods = make(map[string]bool)
}

// ResetCallHistory reinicia los contadores de llamadas
func (r *MockAuthRepository) ResetCallHistory() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callHistory = make(map[string]int)
}

// GetCallCount retorna el número de veces que se ha llamado a un método
func (r *MockAuthRepository) GetCallCount(method string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.callHistory[method]
}

// SetupRefreshTokens inicializa el repositorio con tokens predefinidos
func (r *MockAuthRepository) SetupRefreshTokens(tokens []*entity.RefreshToken) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refreshTokens = make(map[string]*entity.RefreshToken)
	r.tokensByUser = make(map[uuid.UUID][]*entity.RefreshToken)

	for _, token := range tokens {
		clonedToken := r.cloneRefreshToken(token)
		r.refreshTokens[token.Token] = clonedToken
		r.tokensByUser[token.UserID] = append(r.tokensByUser[token.UserID], clonedToken)
	}
}

// SetupFederatedUsers inicializa usuarios federados para pruebas
func (r *MockAuthRepository) SetupFederatedUsers(users []port.UserData) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.federatedUsers = make(map[string]port.UserData)

	for _, user := range users {
		if user.Provider != "" && user.FederatedID != "" {
			key := string(user.Provider) + ":" + user.FederatedID
			r.federatedUsers[key] = user
		}
	}
}

// GetRefreshTokens retorna todos los refresh tokens almacenados
func (r *MockAuthRepository) GetRefreshTokens() []*entity.RefreshToken {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tokens := make([]*entity.RefreshToken, 0, len(r.refreshTokens))
	for _, token := range r.refreshTokens {
		tokens = append(tokens, r.cloneRefreshToken(token))
	}
	return tokens
}

// shouldMethodFail comprueba si un método debería fallar
func (r *MockAuthRepository) shouldMethodFail(method string) bool {
	return r.shouldFail || r.failOnMethods[method]
}

// incrementCallCount incrementa el contador de llamadas para un método
func (r *MockAuthRepository) incrementCallCount(method string) {
	r.callHistory[method] = r.callHistory[method] + 1
}

// cloneRefreshToken crea una copia profunda de un refresh token
func (r *MockAuthRepository) cloneRefreshToken(token *entity.RefreshToken) *entity.RefreshToken {
	return &entity.RefreshToken{
		ID:        token.ID,
		UserID:    token.UserID,
		Token:     token.Token,
		ExpiresAt: token.ExpiresAt,
		CreatedAt: token.CreatedAt,
	}
}

// CreateRefreshToken implementa la interfaz del repositorio
func (r *MockAuthRepository) CreateRefreshToken(ctx context.Context, token *entity.RefreshToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.incrementCallCount("CreateRefreshToken")

	if r.shouldMethodFail("CreateRefreshToken") {
		return ErrMockFailedOp
	}

	// Verificar si ya existe un token con el mismo valor
	if _, exists := r.refreshTokens[token.Token]; exists {
		return ErrMockDuplicatedToken
	}

	// Crear una copia para evitar referencia compartida
	clonedToken := r.cloneRefreshToken(token)
	r.refreshTokens[token.Token] = clonedToken
	r.tokensByUser[token.UserID] = append(r.tokensByUser[token.UserID], clonedToken)

	return nil
}

// GetRefreshToken implementa la interfaz del repositorio
func (r *MockAuthRepository) GetRefreshToken(ctx context.Context, token string) (*entity.RefreshToken, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetRefreshToken")

	if r.shouldMethodFail("GetRefreshToken") {
		return nil, ErrMockFailedOp
	}

	refreshToken, exists := r.refreshTokens[token]
	if !exists {
		return nil, ErrMockTokenNotFound
	}

	return r.cloneRefreshToken(refreshToken), nil
}

// DeleteRefreshToken implementa la interfaz del repositorio
func (r *MockAuthRepository) DeleteRefreshToken(ctx context.Context, token string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.incrementCallCount("DeleteRefreshToken")

	if r.shouldMethodFail("DeleteRefreshToken") {
		return ErrMockFailedOp
	}

	refreshToken, exists := r.refreshTokens[token]
	if !exists {
		return ErrMockTokenNotFound
	}

	// Eliminar del mapa principal
	delete(r.refreshTokens, token)

	// Eliminar del índice por usuario
	userTokens := r.tokensByUser[refreshToken.UserID]
	for i, t := range userTokens {
		if t.Token == token {
			r.tokensByUser[refreshToken.UserID] = append(userTokens[:i], userTokens[i+1:]...)
			break
		}
	}

	return nil
}

// DeleteAllUserRefreshTokens implementa la interfaz del repositorio
func (r *MockAuthRepository) DeleteAllUserRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.incrementCallCount("DeleteAllUserRefreshTokens")

	if r.shouldMethodFail("DeleteAllUserRefreshTokens") {
		return ErrMockFailedOp
	}

	// Obtener todos los tokens del usuario
	userTokens := r.tokensByUser[userID]

	// Eliminar cada token del mapa principal
	for _, token := range userTokens {
		delete(r.refreshTokens, token.Token)
	}

	// Limpiar el índice por usuario
	delete(r.tokensByUser, userID)

	return nil
}

// GetUserByFederatedID implementa la interfaz del repositorio
func (r *MockAuthRepository) GetUserByFederatedID(ctx context.Context, provider value_object.AuthProvider, federatedID string, tenantID *uuid.UUID) (port.UserData, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("GetUserByFederatedID")

	if r.shouldMethodFail("GetUserByFederatedID") {
		return port.UserData{}, ErrMockFailedOp
	}

	key := string(provider) + ":" + federatedID
	user, exists := r.federatedUsers[key]
	if !exists {
		return port.UserData{}, ErrMockUserNotFound
	}

	// Si se especifica tenantID, verificar que coincida
	if tenantID != nil && user.TenantID != *tenantID {
		return port.UserData{}, ErrMockUserNotFound
	}

	return user, nil
}

// LinkFederatedID implementa la interfaz del repositorio
func (r *MockAuthRepository) LinkFederatedID(ctx context.Context, userID uuid.UUID, provider value_object.AuthProvider, federatedID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.incrementCallCount("LinkFederatedID")

	if r.shouldMethodFail("LinkFederatedID") {
		return ErrMockFailedOp
	}

	// En un mock real, aquí actualizaríamos el usuario existente
	// Por simplicidad, solo registramos la llamada
	key := string(provider) + ":" + federatedID

	// Buscar si ya existe un usuario con este userID en federatedUsers
	var existingUser port.UserData
	found := false
	for _, user := range r.federatedUsers {
		if user.ID == userID {
			existingUser = user
			found = true
			break
		}
	}

	if found {
		// Actualizar el usuario existente
		existingUser.Provider = string(provider)
		existingUser.FederatedID = federatedID
		r.federatedUsers[key] = existingUser
	}

	return nil
}

// GetTokenCountByUser retorna el número de tokens activos para un usuario (método auxiliar para tests)
func (r *MockAuthRepository) GetTokenCountByUser(userID uuid.UUID) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tokensByUser[userID])
}
