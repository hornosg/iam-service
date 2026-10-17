package controller_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/hornosg/iam-service/src/tenancy/application/usecase"
	"github.com/hornosg/iam-service/src/tenancy/domain/entity"
	"github.com/hornosg/iam-service/src/tenancy/infrastructure/controller"
	"github.com/hornosg/iam-service/test/tenancy/infrastructure/persistence/repository"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type mockConfigRepo = repository.MockTenantConfigRepository

// injectTenantClaim simula tenantmw.TenantValidation en los tests de controller: copia el header
// X-Tenant-ID (que en producción el middleware valida contra el claim del JWT) al gin context como
// "tenant_id", que es lo que los controllers consumen tras PLAT-E29 T6. Sin header ⇒ sin claim ⇒
// el controller responde 401 (fail-closed), igual que en runtime real sin JWT válido.
func injectTenantClaim() gin.HandlerFunc {
	return func(c *gin.Context) {
		if h := c.GetHeader("X-Tenant-ID"); h != "" {
			c.Set("tenant_id", h)
		}
		c.Next()
	}
}

func setupConfigController(repo *mockConfigRepo) (*controller.TenantConfigController, *gin.Engine) {
	getQ := usecase.NewGetTenantConfigUseCase(repo)
	setCmd := usecase.NewSetTenantConfigUseCase(repo)
	bootCmd := usecase.NewBootstrapTenantConfigUseCase(repo)
	ctrl := controller.NewTenantConfigController(getQ, setCmd, bootCmd)

	r := gin.New()
	r.Use(injectTenantClaim())
	v1 := r.Group("/api/v1")
	ctrl.RegisterReadRoute(v1)
	ctrl.RegisterSetRoute(v1)
	ctrl.RegisterBootstrapRoute(v1)
	return ctrl, r
}

func TestGetConfig_Found(t *testing.T) {
	repo := new(mockConfigRepo)
	_, r := setupConfigController(repo)

	tenantID := uuid.New()
	cfg := entity.NewTenantConfig(tenantID, "catalog.stock_policy", "REQUIRE_STOCK")
	repo.On("GetByKey", mock.Anything, tenantID, "catalog.stock_policy").Return(cfg, true, nil)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/tenant/config/catalog.stock_policy", nil)
	req.Header.Set("X-Tenant-ID", tenantID.String())
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &body)
	assert.Equal(t, "catalog.stock_policy", body["key"])
	assert.Equal(t, "REQUIRE_STOCK", body["value"])
}

func TestGetConfig_NotFound(t *testing.T) {
	repo := new(mockConfigRepo)
	_, r := setupConfigController(repo)

	tenantID := uuid.New()
	repo.On("GetByKey", mock.Anything, tenantID, "catalog.unknown_key").Return(nil, false, nil)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/tenant/config/catalog.unknown_key", nil)
	req.Header.Set("X-Tenant-ID", tenantID.String())
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	var body map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &body)
	assert.Equal(t, "catalog.unknown_key", body["key"])
	assert.Nil(t, body["value"])
}

func TestGetConfig_MissingTenantHeader(t *testing.T) {
	repo := new(mockConfigRepo)
	_, r := setupConfigController(repo)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/tenant/config/catalog.some_key", nil)
	r.ServeHTTP(w, req)

	// ACC-E04 T4 (ADR-005 DD-1): el handler corre tras el gate y lee X-Tenant-ID; ausente ⇒ 400.
	// El 401 sin credencial es del gate Authorize (cubierto en tenant_config_routes_test.go).
	assert.Equal(t, http.StatusBadRequest, w.Code)
	repo.AssertNotCalled(t, "GetByKey", mock.Anything, mock.Anything, mock.Anything)
}

func TestGetConfig_InvalidTenantID(t *testing.T) {
	repo := new(mockConfigRepo)
	_, r := setupConfigController(repo)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/tenant/config/catalog.some_key", nil)
	req.Header.Set("X-Tenant-ID", "not-a-uuid")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetConfig_RepositoryError(t *testing.T) {
	repo := new(mockConfigRepo)
	_, r := setupConfigController(repo)

	tenantID := uuid.New()
	repo.On("GetByKey", mock.Anything, tenantID, "catalog.some_key").Return(nil, false, errors.New("db error"))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/tenant/config/catalog.some_key", nil)
	req.Header.Set("X-Tenant-ID", tenantID.String())
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestSetConfig_Success_NewKey(t *testing.T) {
	repo := new(mockConfigRepo)
	_, r := setupConfigController(repo)

	tenantID := uuid.New()
	repo.On("GetByKey", mock.Anything, tenantID, "catalog.stock_policy").Return(nil, false, nil)
	repo.On("Save", mock.Anything, mock.AnythingOfType("*entity.TenantConfig")).Return(nil)

	body, _ := json.Marshal(map[string]string{"key": "catalog.stock_policy", "value": "REQUIRE_STOCK"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/tenant/config", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", tenantID.String())
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "catalog.stock_policy", resp["key"])
	assert.Equal(t, "REQUIRE_STOCK", resp["value"])
}

func TestSetConfig_MissingTenantHeader(t *testing.T) {
	repo := new(mockConfigRepo)
	_, r := setupConfigController(repo)

	body, _ := json.Marshal(map[string]string{"key": "k", "value": "v"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/tenant/config", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// Set humano: el tenant sale SÓLO del claim (ADR-005 §Camino de escritura); sin claim ⇒ 401.
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSetConfig_InvalidBody(t *testing.T) {
	repo := new(mockConfigRepo)
	_, r := setupConfigController(repo)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/tenant/config", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", uuid.New().String())
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSetConfig_EmptyKey_FailsValidation(t *testing.T) {
	repo := new(mockConfigRepo)
	_, r := setupConfigController(repo)

	body, _ := json.Marshal(map[string]string{"key": "", "value": "v"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/tenant/config", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", uuid.New().String())
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestBootstrapConfig_Success(t *testing.T) {
	repo := new(mockConfigRepo)
	_, r := setupConfigController(repo)

	tenantID := uuid.New()
	// Bootstrap insert-only (ADR-005): sin GetByKey + Save.
	repo.On("InsertMissing", mock.Anything, tenantID, mock.Anything).Return(1, nil)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/tenant/bootstrap", nil)
	req.Header.Set("X-Tenant-ID", tenantID.String())
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &body)
	assert.Equal(t, true, body["success"])
	assert.Equal(t, float64(1), body["configs_created"])
}

func TestBootstrapConfig_Idempotent(t *testing.T) {
	repo := new(mockConfigRepo)
	_, r := setupConfigController(repo)

	tenantID := uuid.New()
	repo.On("InsertMissing", mock.Anything, tenantID, mock.Anything).Return(0, nil)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/tenant/bootstrap", nil)
	req.Header.Set("X-Tenant-ID", tenantID.String())
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &body)
	assert.Equal(t, float64(0), body["configs_created"])
}

func TestBootstrapConfig_MissingTenantHeader(t *testing.T) {
	repo := new(mockConfigRepo)
	_, r := setupConfigController(repo)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/tenant/bootstrap", nil)
	r.ServeHTTP(w, req)

	// ACC-E04 T4 (ADR-005 DD-1): header ausente tras el gate ⇒ 400, sin tocar el repo.
	assert.Equal(t, http.StatusBadRequest, w.Code)
	repo.AssertNotCalled(t, "InsertMissing", mock.Anything, mock.Anything, mock.Anything)
}
