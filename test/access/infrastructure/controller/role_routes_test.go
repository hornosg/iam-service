package controller_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"github.com/hornosg/iam-service/src/identity/infrastructure/adapter"
	"github.com/hornosg/iam-service/src/identity/infrastructure/config"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/hornosg/iam-service/src/access/application/usecase"
	"github.com/hornosg/iam-service/src/access/domain/entity"
	"github.com/hornosg/iam-service/src/access/infrastructure/controller"
	rolecriteria "github.com/hornosg/iam-service/src/access/infrastructure/criteria"
	authmw "github.com/hornosg/iam-service/src/access/infrastructure/middleware"
	"github.com/hornosg/iam-service/src/access/infrastructure/s2s"
	roleMother "github.com/hornosg/iam-service/test/access/domain/entity"
	rolerepo "github.com/hornosg/iam-service/test/access/infrastructure/persistence/repository"
)

const (
	routesTestNS = "mc"
)

// routesTestKey firma los tokens humanos del test (ACC-E03 T6: el gate sólo
// verifica RS256 por kid) y su pública alimenta la keyring del gate.
var routesTestKey = func() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
}()

func routesVerifier() *adapter.JWTServiceAdapter {
	return adapter.NewJWTServiceAdapter(&adapter.SigningKey{PrivateKey: routesTestKey, KID: config.SigningKeyKID(&routesTestKey.PublicKey)})
}

// signRoleToken firma un JWT HS256 con namespace y roles dados.
func signRoleToken(t *testing.T, roles []string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"iss":       "iam-service", // ACC-E09 T2: todo token de usuario real lo lleva
		"namespace": routesTestNS,
		"user_id":   "123e4567-e89b-12d3-a456-426614174000",
		"tenant_id": "123e4567-e89b-12d3-a456-426614174003",
		"roles":     roles,
		"exp":       time.Now().Add(time.Hour).Unix(),
	}
	jt := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	jt.Header["kid"] = config.SigningKeyKID(&routesTestKey.PublicKey)
	tok, err := jt.SignedString(routesTestKey)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return tok
}

// newRolesRouter levanta un router que cablea el módulo role igual que main.go:
// lectura en tenantScopedGroup (system:admin ∪ tenant:admin), escritura en
// adminGroup (system:admin). Usa un mock repo, no DB.
func newRolesRouter(t *testing.T) (*gin.Engine, *rolerepo.MockRoleRepository) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery())

	// Registry S2S vacío: en estos tests autorizamos sólo por JWT.
	registry := s2s.LoadFromEnvForTests(map[string]string{})
	authFactory := authmw.NewScopeMiddlewareFactory(routesTestNS, registry, routesVerifier())

	apiV1 := router.Group("/api/v1")
	adminGroup := apiV1.Group("", authFactory.RequireScope(s2s.ScopeSystemAdmin, "system_admin"))
	tenantScopedGroup := apiV1.Group("", authFactory.RequireScopes(
		[]s2s.Scope{s2s.ScopeSystemAdmin, s2s.ScopeTenantAdmin}, "tenant_admin", "system_admin"))

	mockRepo := rolerepo.NewMockRoleRepository()
	roleHandler := controller.NewRoleHandler(
		usecase.NewCreateRoleUseCase(mockRepo),
		usecase.NewGetRoleByIDUseCase(mockRepo),
		usecase.NewUpdateRoleUseCase(mockRepo),
		usecase.NewDeleteRoleUseCase(mockRepo),
		usecase.NewListRolesUseCase(mockRepo),
		usecase.NewListRolesByCriteriaUseCase(mockRepo),
		rolecriteria.NewRoleCriteriaBuilder(),
	)
	roleHandler.RegisterRoutes(tenantScopedGroup, adminGroup)

	return router, mockRepo
}

func doRequest(t *testing.T, router *gin.Engine, method, path, token string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		buf.Write(b)
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// --- Criterio (b): escritura gated por system:admin ---

func TestRoleRoutes_POST_TenantAdmin_Forbidden(t *testing.T) {
	router, _ := newRolesRouter(t)
	w := doRequest(t, router, http.MethodPost, "/api/v1/roles",
		signRoleToken(t, []string{"tenant_admin"}),
		map[string]interface{}{"name": "X", "description": "Desc valido", "type": "CUSTOM"})
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRoleRoutes_POST_SystemAdmin_Created(t *testing.T) {
	router, _ := newRolesRouter(t)
	w := doRequest(t, router, http.MethodPost, "/api/v1/roles",
		signRoleToken(t, []string{"system_admin"}),
		map[string]interface{}{"name": "Nuevo Rol", "description": "Desc valido de prueba", "type": "CUSTOM"})
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestRoleRoutes_PUT_TenantAdmin_Forbidden(t *testing.T) {
	router, _ := newRolesRouter(t)
	w := doRequest(t, router, http.MethodPut, "/api/v1/roles/"+uuid.New().String(),
		signRoleToken(t, []string{"tenant_admin"}),
		map[string]interface{}{"name": "Y", "description": "Desc valido"})
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRoleRoutes_PUT_SystemAdmin_NotForbiddenByGate(t *testing.T) {
	router, _ := newRolesRouter(t)
	// El gate system:admin pasa (no 403). El handler puede 404/200 según el id;
	// lo que se verifica acá es que el gate NO bloquea a system:admin.
	w := doRequest(t, router, http.MethodPut, "/api/v1/roles/"+uuid.New().String(),
		signRoleToken(t, []string{"system_admin"}),
		map[string]interface{}{"name": "Y", "description": "Desc valido"})
	assert.NotEqual(t, http.StatusForbidden, w.Code)
}

func TestRoleRoutes_DELETE_TenantAdmin_Forbidden(t *testing.T) {
	router, _ := newRolesRouter(t)
	w := doRequest(t, router, http.MethodDelete, "/api/v1/roles/"+uuid.New().String(),
		signRoleToken(t, []string{"tenant_admin"}), nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// --- Criterio (c): lectura legible por tenant:admin y system:admin ---

func TestRoleRoutes_GET_List_TenantAdmin_OK(t *testing.T) {
	router, mockRepo := newRolesRouter(t)
	mockRepo.SetupRoles([]*entity.Role{roleMother.Create().Custom()})
	w := doRequest(t, router, http.MethodGet, "/api/v1/roles",
		signRoleToken(t, []string{"tenant_admin"}), nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRoleRoutes_GET_List_SystemAdmin_OK(t *testing.T) {
	router, _ := newRolesRouter(t)
	w := doRequest(t, router, http.MethodGet, "/api/v1/roles",
		signRoleToken(t, []string{"system_admin"}), nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRoleRoutes_GET_ByID_TenantAdmin_OK(t *testing.T) {
	router, mockRepo := newRolesRouter(t)
	role := roleMother.Create().Custom()
	mockRepo.SetupRoles([]*entity.Role{role})

	w := doRequest(t, router, http.MethodGet, "/api/v1/roles/"+role.ID.String(),
		signRoleToken(t, []string{"tenant_admin"}), nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRoleRoutes_GET_ByID_SystemAdmin_OK(t *testing.T) {
	router, mockRepo := newRolesRouter(t)
	role := roleMother.Create().Custom()
	mockRepo.SetupRoles([]*entity.Role{role})

	w := doRequest(t, router, http.MethodGet, "/api/v1/roles/"+role.ID.String(),
		signRoleToken(t, []string{"system_admin"}), nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

// --- Sin token: fail-closed (la defensa es el gate, no Kong) ---

func TestRoleRoutes_POST_NoToken_Unauthorized(t *testing.T) {
	router, _ := newRolesRouter(t)
	w := doRequest(t, router, http.MethodPost, "/api/v1/roles", "",
		map[string]interface{}{"name": "X", "description": "Desc", "type": "CUSTOM"})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
