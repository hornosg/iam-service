package controller_test

// ACC-E08 T3: tests de traducción HTTP del AuthHandler (src/identity/infrastructure/
// controller/http_handler.go) — request binding → usecase → response/status. Los usecases
// corren REALES con mocks de sus ports (mismo patrón que test/access/infrastructure/
// controller y test/identity/auth/application/usecase): lo que se verifica es el mapeo
// de error del usecase a status HTTP, no la lógica de negocio.

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/hornosg/iam-service/src/identity/application/usecase"
	domentity "github.com/hornosg/iam-service/src/identity/domain/entity"
	"github.com/hornosg/iam-service/src/identity/domain/port"
	auth_vo "github.com/hornosg/iam-service/src/identity/domain/value_object"
	"github.com/hornosg/iam-service/src/identity/infrastructure/adapter"
	"github.com/hornosg/iam-service/src/identity/infrastructure/config"
	"github.com/hornosg/iam-service/src/identity/infrastructure/controller"
	sharedlog "github.com/hornosg/go-shared/infrastructure/logging"
	authEntity "github.com/hornosg/iam-service/test/identity/auth/domain/entity"
	authrepo "github.com/hornosg/iam-service/test/identity/auth/infrastructure/persistence/repository"
)

// --- Material de firma: RS256+kid (ADR-003) desde el flip del firmador ---

var (
	testSigningKeyOnce sync.Once
	testSigningKey     *adapter.SigningKey
)

func newTestSigningKey() *adapter.SigningKey {
	testSigningKeyOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		testSigningKey = &adapter.SigningKey{
			PrivateKey: key,
			KID:        config.SigningKeyKID(&key.PublicKey),
		}
	})
	return testSigningKey
}

func newTestJWTService() *adapter.JWTServiceAdapter {
	return adapter.NewJWTServiceAdapter("test-secret-key-at-least-32-chars-long!!", newTestSigningKey())
}

// --- Mocks de los ports que los usecases de auth necesitan ---

type MockUserService struct {
	users      map[uuid.UUID]*port.UserData
	shouldFail bool
}

func NewMockUserService() *MockUserService {
	return &MockUserService{users: make(map[uuid.UUID]*port.UserData)}
}

func (m *MockUserService) SetupUser(user *port.UserData) { m.users[user.ID] = user }

func (m *MockUserService) FindUserByID(_ context.Context, id uuid.UUID) (*port.UserData, error) {
	if m.shouldFail {
		return nil, usecase.ErrUserNotFound
	}
	user, exists := m.users[id]
	if !exists {
		return nil, usecase.ErrUserNotFound
	}
	return user, nil
}

func (m *MockUserService) FindUserByEmail(_ context.Context, email string, tenantID *uuid.UUID) (*port.UserData, error) {
	if m.shouldFail {
		return nil, usecase.ErrUserNotFound
	}
	for _, user := range m.users {
		if user.Email == email {
			if tenantID == nil || user.TenantID == *tenantID {
				return user, nil
			}
		}
	}
	return nil, usecase.ErrUserNotFound
}

type MockTenantService struct{}

func (m *MockTenantService) Execute(_ context.Context, _ uuid.UUID) (*auth_vo.TenantFeatures, error) {
	return &auth_vo.TenantFeatures{FriendsFamily: true}, nil
}

type MockRoleResolver struct{}

func (m *MockRoleResolver) GetRole(_ context.Context, _ uuid.UUID) (*port.ResolvedRole, error) {
	return &port.ResolvedRole{Slug: "user", Permissions: []string{"user:read"}, IsActive: true}, nil
}

type MockPlanResolver struct{}

func (m *MockPlanResolver) GetPlanForTenant(_ context.Context, _ uuid.UUID) (*port.ResolvedPlan, error) {
	return &port.ResolvedPlan{Tier: "FREE", IsActive: true}, nil
}

type MockGoogleTokenVerifier struct {
	claims port.GoogleClaims
	err    error
}

func (m *MockGoogleTokenVerifier) SetupClaims(claims port.GoogleClaims) { m.claims = claims }

func (m *MockGoogleTokenVerifier) Verify(_ context.Context, _ string) (port.GoogleClaims, error) {
	return m.claims, m.err
}

const authTestSecurityLogger = "iam-controller-test"

// newAuthHandler arma el AuthHandler con usecases REALES sobre mocks. Los
// repos pre/post auth son el mismo mock salvo que el test pase uno distinto
// para el post-auth (para inyectar el fallo de la fase de emisión de tokens).
func newAuthHandler(
	preAuth, postAuth *authrepo.MockAuthRepository,
	userService *MockUserService,
	googleVerifier *MockGoogleTokenVerifier,
) *controller.AuthHandler {
	if postAuth == nil {
		postAuth = preAuth
	}
	if googleVerifier == nil {
		googleVerifier = &MockGoogleTokenVerifier{}
	}
	config := usecase.AuthConfig{
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		Namespace:          "mc",
	}
	return controller.NewAuthHandler(
		usecase.NewLoginUseCase(config, preAuth, postAuth, userService, &MockTenantService{},
			newTestJWTService(), &MockRoleResolver{}, &MockPlanResolver{}, googleVerifier,
			sharedlog.NewSecurityLoggerWithWriter(authTestSecurityLogger, io.Discard)),
		usecase.NewRefreshTokenUseCase(config, postAuth, userService, &MockTenantService{},
			newTestJWTService(), &MockRoleResolver{}, &MockPlanResolver{}),
		usecase.NewLogoutUseCase(postAuth, sharedlog.NewSecurityLoggerWithWriter(authTestSecurityLogger, io.Discard)),
		usecase.NewRevokeAllUseCase(postAuth, 15*time.Minute,
			sharedlog.NewSecurityLoggerWithWriter(authTestSecurityLogger, io.Discard)),
	)
}

func newAuthRouter(h *controller.AuthHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery())
	apiV1 := router.Group("/api/v1")
	h.RegisterRoutes(apiV1)
	return router
}

func doAuthRequest(t *testing.T, router *gin.Engine, method, path string, body interface{}, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		buf.Write(b)
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func localUser(t *testing.T, email, password string) *port.UserData {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	require.NoError(t, err)
	return &port.UserData{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: string(hash),
		TenantID:     uuid.New(),
		RoleID:       uuid.New(),
		Status:       "ACTIVE",
		Provider:     "LOCAL",
	}
}

// --- Login ---

func TestAuthRoutes_Login_OK(t *testing.T) {
	user := localUser(t, "login-ok@test.com", "password123")
	userSvc := NewMockUserService()
	userSvc.SetupUser(user)
	mockRepo := authrepo.NewMockAuthRepository()
	router := newAuthRouter(newAuthHandler(mockRepo, nil, userSvc, nil))

	w := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/login", map[string]interface{}{
		"email": "login-ok@test.com", "password": "password123", "provider": "LOCAL",
		"tenant_id": user.TenantID.String(),
	}, nil)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["access_token"])
	assert.NotEmpty(t, resp["refresh_token"])
}

func TestAuthRoutes_Login_BadBody_BadRequest(t *testing.T) {
	router := newAuthRouter(newAuthHandler(authrepo.NewMockAuthRepository(), nil, NewMockUserService(), nil))

	// Falta provider (binding required) → 400 con detalle al log, no al cuerpo.
	w := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/login",
		map[string]interface{}{"email": "x@test.com", "password": "password123"}, nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Datos de entrada inválidos")
	assert.NotContains(t, w.Body.String(), "Provider") // ACC-E02 T8j: detalle no va al cuerpo
}

func TestAuthRoutes_Login_InvalidCredentials_Unauthorized(t *testing.T) {
	user := localUser(t, "login-bad@test.com", "password123")
	userSvc := NewMockUserService()
	userSvc.SetupUser(user)
	router := newAuthRouter(newAuthHandler(authrepo.NewMockAuthRepository(), nil, userSvc, nil))

	w := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/login", map[string]interface{}{
		"email": "login-bad@test.com", "password": "otra-password", "provider": "LOCAL",
	}, nil)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Credenciales inválidas")
}

func TestAuthRoutes_Login_Google_UserNotFound_Unauthorized(t *testing.T) {
	// Google con usuario inexistente: ni por federatedID ni por email →
	// ErrUserNotFound → 401 "Usuario no encontrado" (rama propia del handler).
	verifier := &MockGoogleTokenVerifier{}
	verifier.SetupClaims(port.GoogleClaims{Sub: "google-sub-1", Email: "nobody@test.com"})
	router := newAuthRouter(newAuthHandler(authrepo.NewMockAuthRepository(), nil, NewMockUserService(), verifier))

	w := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/login", map[string]interface{}{
		// El binding del LoginRequest exige email incluso en el flujo Google
		// (tag required): sin él, ni siquiera llega al usecase.
		"email": "nobody@test.com", "provider": "GOOGLE", "google_token": "id-token",
	}, nil)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Usuario no encontrado")
}

func TestAuthRoutes_Login_TokenGenerationFails_InternalError(t *testing.T) {
	// Auth OK pero el post-auth falla al crear el refresh token → error no
	// centinela → 500 (el detalle interno va al log, no al cuerpo).
	user := localUser(t, "login-500@test.com", "password123")
	userSvc := NewMockUserService()
	userSvc.SetupUser(user)
	preAuth := authrepo.NewMockAuthRepository()
	postAuth := authrepo.NewMockAuthRepository()
	postAuth.ShouldFailOn("CreateRefreshToken")
	router := newAuthRouter(newAuthHandler(preAuth, postAuth, userSvc, nil))

	w := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/login", map[string]interface{}{
		"email": "login-500@test.com", "password": "password123", "provider": "LOCAL",
	}, nil)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Error interno del servidor")
	assert.NotContains(t, w.Body.String(), "generando")
}

// --- RefreshToken ---

func seedRefreshToken(repo *authrepo.MockAuthRepository, userID, tenantID uuid.UUID, token string, expiresAt time.Time) {
	rt := authEntity.Create().WithUser(userID)
	rt.Token = token
	rt.TenantID = tenantID
	rt.ExpiresAt = expiresAt
	repo.SetupRefreshTokens([]*domentity.RefreshToken{rt})
}

func TestAuthRoutes_Refresh_OK(t *testing.T) {
	userID, tenantID := uuid.New(), uuid.New()
	userSvc := NewMockUserService()
	userSvc.SetupUser(&port.UserData{ID: userID, Email: "refresh@test.com", TenantID: tenantID,
		RoleID: uuid.New(), Status: "ACTIVE"})
	mockRepo := authrepo.NewMockAuthRepository()
	seedRefreshToken(mockRepo, userID, tenantID, "valid_refresh_token", time.Now().Add(time.Hour))
	router := newAuthRouter(newAuthHandler(mockRepo, nil, userSvc, nil))

	w := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/refresh",
		map[string]interface{}{"refresh_token": "valid_refresh_token"}, nil)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["access_token"])
	assert.NotEmpty(t, resp["refresh_token"])
}

func TestAuthRoutes_Refresh_BadBody_BadRequest(t *testing.T) {
	router := newAuthRouter(newAuthHandler(authrepo.NewMockAuthRepository(), nil, NewMockUserService(), nil))

	w := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/refresh",
		map[string]interface{}{}, nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Refresh token requerido")
}

func TestAuthRoutes_Refresh_InvalidToken_Unauthorized(t *testing.T) {
	router := newAuthRouter(newAuthHandler(authrepo.NewMockAuthRepository(), nil, NewMockUserService(), nil))

	w := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/refresh",
		map[string]interface{}{"refresh_token": "token_inexistente"}, nil)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Refresh token inválido")
}

func TestAuthRoutes_Refresh_ExpiredToken_Unauthorized(t *testing.T) {
	userID, tenantID := uuid.New(), uuid.New()
	mockRepo := authrepo.NewMockAuthRepository()
	seedRefreshToken(mockRepo, userID, tenantID, "expired_refresh_token", time.Now().Add(-time.Hour))
	router := newAuthRouter(newAuthHandler(mockRepo, nil, NewMockUserService(), nil))

	w := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/refresh",
		map[string]interface{}{"refresh_token": "expired_refresh_token"}, nil)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Refresh token expirado")
}

func TestAuthRoutes_Refresh_UserNotFound_Unauthorized(t *testing.T) {
	// Token válido pero el usuario ya no existe → ErrUserNotFound → 401.
	mockRepo := authrepo.NewMockAuthRepository()
	seedRefreshToken(mockRepo, uuid.New(), uuid.New(), "orphan_refresh_token", time.Now().Add(time.Hour))
	router := newAuthRouter(newAuthHandler(mockRepo, nil, NewMockUserService(), nil))

	w := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/refresh",
		map[string]interface{}{"refresh_token": "orphan_refresh_token"}, nil)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Usuario no encontrado")
}

func TestAuthRoutes_Refresh_WithPresenterBearer_OK(t *testing.T) {
	// ACC-E02 T8e: el Bearer es OPCIONAL. Si viene y es ilegible no bloquea:
	// el refresh sigue con la posesión del refresh token como credencial.
	userID, tenantID := uuid.New(), uuid.New()
	userSvc := NewMockUserService()
	userSvc.SetupUser(&port.UserData{ID: userID, Email: "presenter@test.com", TenantID: tenantID,
		RoleID: uuid.New(), Status: "ACTIVE"})
	mockRepo := authrepo.NewMockAuthRepository()
	seedRefreshToken(mockRepo, userID, tenantID, "presenter_refresh_token", time.Now().Add(time.Hour))
	router := newAuthRouter(newAuthHandler(mockRepo, nil, userSvc, nil))

	w := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/refresh",
		map[string]interface{}{"refresh_token": "presenter_refresh_token"},
		map[string]string{"Authorization": "Bearer token-ilegible-que-no-parsea"})

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthRoutes_Refresh_BearerCrossTenant_Unauthorized(t *testing.T) {
	// El Bearer parsea y reclama OTRO tenant que la fila del refresh token →
	// par (access, refresh) inconsistente → 401 (defensa anti cross-tenant).
	userID, tenantID := uuid.New(), uuid.New()
	userSvc := NewMockUserService()
	userSvc.SetupUser(&port.UserData{ID: userID, Email: "cross@test.com", TenantID: tenantID,
		RoleID: uuid.New(), Status: "ACTIVE"})
	mockRepo := authrepo.NewMockAuthRepository()
	seedRefreshToken(mockRepo, userID, tenantID, "cross_refresh_token", time.Now().Add(time.Hour))
	router := newAuthRouter(newAuthHandler(mockRepo, nil, userSvc, nil))

	otherTenant := uuid.New()
	presenterClaims := auth_vo.NewTokenClaims(userID, otherTenant, uuid.New(), "x@test.com", "mc",
		nil, time.Now().Add(time.Hour))
	presenterToken, err := newTestJWTService().Sign(presenterClaims)
	require.NoError(t, err)

	w := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/refresh",
		map[string]interface{}{"refresh_token": "cross_refresh_token"},
		map[string]string{"Authorization": "Bearer " + presenterToken})

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Refresh token inválido")
}

func TestAuthRoutes_Refresh_DeleteFails_InternalError(t *testing.T) {
	// Fallo de motor al borrar el refresh token (no es "ya consumido") →
	// error crudo → 500 default del handler.
	userID, tenantID := uuid.New(), uuid.New()
	userSvc := NewMockUserService()
	userSvc.SetupUser(&port.UserData{ID: userID, Email: "del@test.com", TenantID: tenantID,
		RoleID: uuid.New(), Status: "ACTIVE"})
	mockRepo := authrepo.NewMockAuthRepository()
	seedRefreshToken(mockRepo, userID, tenantID, "del_refresh_token", time.Now().Add(time.Hour))
	mockRepo.ShouldFailOn("DeleteRefreshToken")
	router := newAuthRouter(newAuthHandler(mockRepo, nil, userSvc, nil))

	w := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/refresh",
		map[string]interface{}{"refresh_token": "del_refresh_token"}, nil)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Error interno del servidor")
}

// --- Logout / RevokeAll ---

func setAuthContext(userID uuid.UUID, claims *auth_vo.TokenClaims) gin.HandlerFunc {
	return func(c *gin.Context) {
		if userID != uuid.Nil {
			c.Set("user_id", userID)
		}
		if claims != nil {
			c.Set("token_claims", claims)
		}
		c.Next()
	}
}

func TestAuthRoutes_Logout_NoUserID_Unauthorized(t *testing.T) {
	router := newAuthRouter(newAuthHandler(authrepo.NewMockAuthRepository(), nil, NewMockUserService(), nil))

	w := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/logout", nil, nil)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Usuario no autenticado")
}

func TestAuthRoutes_Logout_WrongUserIDType_InternalError(t *testing.T) {
	// user_id presente pero no es uuid.UUID (middleware roto) → 500.
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := newAuthHandler(authrepo.NewMockAuthRepository(), nil, NewMockUserService(), nil)
	apiV1 := router.Group("/api/v1")
	apiV1.Use(func(c *gin.Context) { c.Set("user_id", "no-soy-un-uuid"); c.Next() })
	h.RegisterRoutes(apiV1)

	w := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/logout", nil, nil)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "ID de usuario inválido")
}

func TestAuthRoutes_Logout_OK_NoContent(t *testing.T) {
	userID := uuid.New()
	mockRepo := authrepo.NewMockAuthRepository()
	// Con token_claims presente: además del corte de refresh tokens, revoca el
	// access token vigente (JTI). Sin claims: sólo DeleteAllUserRefreshTokens.
	claims := auth_vo.NewTokenClaims(userID, uuid.New(), uuid.New(), "l@test.com", "mc",
		nil, time.Now().Add(time.Hour))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(setAuthContext(userID, claims))
	newAuthHandler(mockRepo, nil, NewMockUserService(), nil).RegisterRoutes(router.Group("/api/v1"))

	w := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/logout", nil, nil)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, 1, mockRepo.GetCallCount("RevokeToken"))
	assert.Equal(t, 1, mockRepo.GetCallCount("DeleteAllUserRefreshTokens"))
}

func TestAuthRoutes_Logout_RepoFails_InternalError(t *testing.T) {
	userID := uuid.New()
	mockRepo := authrepo.NewMockAuthRepository()
	mockRepo.ShouldFailOn("DeleteAllUserRefreshTokens")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(setAuthContext(userID, nil))
	newAuthHandler(mockRepo, nil, NewMockUserService(), nil).RegisterRoutes(router.Group("/api/v1"))

	w := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/logout", nil, nil)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Error cerrando sesión")
	assert.NotContains(t, w.Body.String(), "operación") // detalle al log (ACC-E02 T8g)
}

func TestAuthRoutes_RevokeAll_OK(t *testing.T) {
	userID := uuid.New()
	mockRepo := authrepo.NewMockAuthRepository()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(setAuthContext(userID, nil))
	newAuthHandler(mockRepo, nil, NewMockUserService(), nil).RegisterRoutes(router.Group("/api/v1"))

	w := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/revoke-all", nil, nil)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, mockRepo.GetCallCount("RevokeAllUserTokens"))
	assert.Equal(t, 1, mockRepo.GetCallCount("DeleteAllUserRefreshTokens"))
}

func TestAuthRoutes_RevokeAll_NoUserID_Unauthorized(t *testing.T) {
	router := newAuthRouter(newAuthHandler(authrepo.NewMockAuthRepository(), nil, NewMockUserService(), nil))

	w := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/revoke-all", nil, nil)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Usuario no autenticado")
}

func TestAuthRoutes_RevokeAll_WrongUserIDType_InternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := newAuthHandler(authrepo.NewMockAuthRepository(), nil, NewMockUserService(), nil)
	apiV1 := router.Group("/api/v1")
	apiV1.Use(func(c *gin.Context) { c.Set("user_id", 12345); c.Next() })
	h.RegisterRoutes(apiV1)

	w := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/revoke-all", nil, nil)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "ID de usuario inválido")
}

func TestAuthRoutes_RevokeAll_RepoFails_InternalError(t *testing.T) {
	userID := uuid.New()
	mockRepo := authrepo.NewMockAuthRepository()
	mockRepo.ShouldFailOn("RevokeAllUserTokens")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(setAuthContext(userID, nil))
	newAuthHandler(mockRepo, nil, NewMockUserService(), nil).RegisterRoutes(router.Group("/api/v1"))

	w := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/revoke-all", nil, nil)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Error revocando tokens")
}