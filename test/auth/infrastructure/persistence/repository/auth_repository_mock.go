package repository

import (
	"context"
	"errors"
	"sync"
	"time"

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
	revokedTokens  map[uuid.UUID]time.Time  // jti -> expiresAt
	// userCuts replica las marcas scope='user' de revoke-all (T8i):
	// userID -> momento del corte. IsTokenRevoked la coteja contra el iat.
	userCuts map[uuid.UUID]time.Time
	shouldFail     bool
	failOnMethods  map[string]bool
	methodErrors   map[string]error // error específico inyectado por método (p. ej. sentinelas del dominio)
	callHistory    map[string]int
}

// NewMockAuthRepository crea una nueva instancia del mock
func NewMockAuthRepository() *MockAuthRepository {
	return &MockAuthRepository{
		refreshTokens:  make(map[string]*entity.RefreshToken),
		tokensByUser:   make(map[uuid.UUID][]*entity.RefreshToken),
		federatedUsers: make(map[string]port.UserData),
		revokedTokens:  make(map[uuid.UUID]time.Time),
		userCuts:       make(map[uuid.UUID]time.Time),
		failOnMethods:  make(map[string]bool),
		methodErrors:   make(map[string]error),
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

// FailMethodWith inyecta un error ESPECÍFICO para un método — para sentinelas
// del dominio que el caso de uso distingue con errors.Is (T8f:
// port.ErrRefreshTokenAlreadyConsumed). ErrMockFailedOp no sirve para eso.
func (r *MockAuthRepository) FailMethodWith(method string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.methodErrors[method] = err
}

// ResetFailures limpia todas las configuraciones de fallo
func (r *MockAuthRepository) ResetFailures() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shouldFail = false
	r.failOnMethods = make(map[string]bool)
	r.methodErrors = make(map[string]error)
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

// cloneRefreshToken crea una copia profunda de un refresh token. T8e: la fila
// trae tenant_id denormalizado (NOT NULL desde la migración 021) — el clone lo
// conserva, porque el usecase lo exige para abrir el contexto de RLS.
func (r *MockAuthRepository) cloneRefreshToken(token *entity.RefreshToken) *entity.RefreshToken {
	return &entity.RefreshToken{
		ID:        token.ID,
		UserID:    token.UserID,
		TenantID:  token.TenantID,
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

	if err := r.methodErrors["DeleteRefreshToken"]; err != nil {
		return err
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

// RevokeToken implementa la interfaz del repositorio
func (r *MockAuthRepository) RevokeToken(ctx context.Context, jti uuid.UUID, userID uuid.UUID, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.incrementCallCount("RevokeToken")

	if r.shouldMethodFail("RevokeToken") {
		return ErrMockFailedOp
	}

	r.revokedTokens[jti] = expiresAt
	return nil
}

// IsTokenRevoked implementa la interfaz del repositorio. Replica la semántica
// real del gate (T8i): revocado por JTI o por marca scope='user' con
// revoked_at > issuedAt (issuedAt=0 → fail-closed, cualquier marca viva
// revoca).
func (r *MockAuthRepository) IsTokenRevoked(ctx context.Context, jti uuid.UUID, userID uuid.UUID, issuedAt int64) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.incrementCallCount("IsTokenRevoked")

	if r.shouldMethodFail("IsTokenRevoked") {
		return false, ErrMockFailedOp
	}

	if _, exists := r.revokedTokens[jti]; exists {
		return true, nil
	}
	if cutAt, ok := r.userCuts[userID]; ok && issuedAt < cutAt.Unix() {
		return true, nil
	}
	return false, nil
}

// RevokeAllUserTokens implementa la interfaz del repositorio
func (r *MockAuthRepository) RevokeAllUserTokens(ctx context.Context, userID uuid.UUID, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.incrementCallCount("RevokeAllUserTokens")

	if r.shouldMethodFail("RevokeAllUserTokens") {
		return ErrMockFailedOp
	}

	r.userCuts[userID] = time.Now()
	return nil
}

// CleanupExpiredRevocations implementa la interfaz del repositorio
func (r *MockAuthRepository) CleanupExpiredRevocations(ctx context.Context) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.incrementCallCount("CleanupExpiredRevocations")

	if r.shouldMethodFail("CleanupExpiredRevocations") {
		return 0, ErrMockFailedOp
	}

	var count int64
	now := time.Now()
	for jti, expiresAt := range r.revokedTokens {
		if expiresAt.Before(now) {
			delete(r.revokedTokens, jti)
			count++
		}
	}
	return count, nil
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
