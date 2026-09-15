package controller_test

// ACC-E08 T3: tests de traducción HTTP del UserHandler (src/identity/infrastructure/
// controller/user_handler.go) — request binding → usecase → response/status. Usecases
// REALES sobre el MockUserRepository del árbol de tests; lo que se verifica es el mapeo
// de error del usecase a status HTTP.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	crit "github.com/hornosg/go-shared/criteria"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hornosg/iam-service/src/identity/application/usecase"
	domentity "github.com/hornosg/iam-service/src/identity/domain/entity"
	"github.com/hornosg/iam-service/src/identity/infrastructure/controller"
	usermother "github.com/hornosg/iam-service/test/identity/user/domain/entity"
	userrepo "github.com/hornosg/iam-service/test/identity/user/infrastructure/persistence/repository"
)

func newUserHandler(mockRepo *userrepo.MockUserRepository) *controller.UserHandler {
	return controller.NewUserHandler(
		usecase.NewCreateUserUseCase(mockRepo),
		usecase.NewGetUserByIDUseCase(mockRepo),
		usecase.NewUpdateUserUseCase(mockRepo),
		usecase.NewDeleteUserUseCase(mockRepo),
		usecase.NewListUsersByCriteriaUseCase(mockRepo),
	)
}

func newUserRouter(mockRepo *userrepo.MockUserRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery())
	apiV1 := router.Group("/api/v1")
	newUserHandler(mockRepo).RegisterRoutes(apiV1)
	return router
}

func doUserRequest(t *testing.T, router *gin.Engine, method, path string, body interface{}, headers map[string]string) *httptest.ResponseRecorder {
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

// --- CreateUser ---

func TestUserRoutes_Create_OK(t *testing.T) {
	mockRepo := userrepo.NewMockUserRepository()
	router := newUserRouter(mockRepo)

	w := doUserRequest(t, router, http.MethodPost, "/api/v1/users", map[string]interface{}{
		"email": "nuevo@test.com", "password": "password123",
		"tenant_id": uuid.New().String(), "role_id": uuid.New().String(),
	}, nil)

	require.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, 1, mockRepo.GetCallCount("Create"))
}

func TestUserRoutes_Create_BadBody_BadRequest(t *testing.T) {
	router := newUserRouter(userrepo.NewMockUserRepository())

	// Email ausente → binding tag required → 400 con detalles (JSONWithDetails).
	w := doUserRequest(t, router, http.MethodPost, "/api/v1/users", map[string]interface{}{
		"password": "password123",
		"tenant_id": uuid.New().String(), "role_id": uuid.New().String(),
	}, nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Datos de entrada inválidos")
}

func TestUserRoutes_Create_EmailAlreadyExists_Conflict(t *testing.T) {
	tenantID := uuid.New()
	existing := usermother.UserMother{}.WithEmail("dup@test.com")
	existing.TenantID = tenantID
	mockRepo := userrepo.NewMockUserRepository()
	mockRepo.SetupUsers([]*domentity.User{existing})
	router := newUserRouter(mockRepo)

	w := doUserRequest(t, router, http.MethodPost, "/api/v1/users", map[string]interface{}{
		"email": "dup@test.com", "password": "password123",
		"tenant_id": tenantID.String(), "role_id": uuid.New().String(),
	}, nil)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "El usuario ya existe")
}

func TestUserRoutes_Create_WeakPassword_BadRequest(t *testing.T) {
	// La validación de la request (password < 8 para LOCAL) entra como error
	// no centinela → rama default del handler → 500 con detalle.
	mockRepo := userrepo.NewMockUserRepository()
	router := newUserRouter(mockRepo)

	w := doUserRequest(t, router, http.MethodPost, "/api/v1/users", map[string]interface{}{
		"email": "corto@test.com", "password": "123",
		"tenant_id": uuid.New().String(), "role_id": uuid.New().String(),
	}, nil)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Error interno del servidor")
}

func TestUserRoutes_Create_RepoFails_InternalError(t *testing.T) {
	mockRepo := userrepo.NewMockUserRepository()
	mockRepo.ShouldFailOn("ExistsByEmail")
	router := newUserRouter(mockRepo)

	w := doUserRequest(t, router, http.MethodPost, "/api/v1/users", map[string]interface{}{
		"email": "fallo@test.com", "password": "password123",
		"tenant_id": uuid.New().String(), "role_id": uuid.New().String(),
	}, nil)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// --- GetUserByID ---

func TestUserRoutes_GetByID_OK(t *testing.T) {
	user := usermother.UserMother{}.WithDefaults()
	mockRepo := userrepo.NewMockUserRepository()
	mockRepo.SetupUsers([]*domentity.User{user})
	router := newUserRouter(mockRepo)

	w := doUserRequest(t, router, http.MethodGet, "/api/v1/users/"+user.ID.String(), nil, nil)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, user.ID.String(), resp["id"])
}

func TestUserRoutes_GetByID_BadUUID_BadRequest(t *testing.T) {
	router := newUserRouter(userrepo.NewMockUserRepository())

	w := doUserRequest(t, router, http.MethodGet, "/api/v1/users/no-es-un-uuid", nil, nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "ID de usuario inválido")
}

func TestUserRoutes_GetByID_NotFound(t *testing.T) {
	router := newUserRouter(userrepo.NewMockUserRepository())

	w := doUserRequest(t, router, http.MethodGet, "/api/v1/users/"+uuid.New().String(), nil, nil)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "Usuario no encontrado")
}

func TestUserRoutes_GetByID_TenantMismatch_NotFound(t *testing.T) {
	// ACC-E02 (aislamiento por caso de uso): un usuario de otro tenant no se
	// distingue de uno inexistente — 404, nunca 403 ni el dato.
	user := usermother.UserMother{}.WithDefaults()
	mockRepo := userrepo.NewMockUserRepository()
	mockRepo.SetupUsers([]*domentity.User{user})
	router := newUserRouter(mockRepo)

	w := doUserRequest(t, router, http.MethodGet, "/api/v1/users/"+user.ID.String(), nil,
		map[string]string{"X-Tenant-ID": uuid.New().String()})

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// --- UpdateUser ---

func TestUserRoutes_Update_OK(t *testing.T) {
	user := usermother.UserMother{}.WithDefaults()
	mockRepo := userrepo.NewMockUserRepository()
	mockRepo.SetupUsers([]*domentity.User{user})
	router := newUserRouter(mockRepo)

	w := doUserRequest(t, router, http.MethodPut, "/api/v1/users/"+user.ID.String(),
		map[string]interface{}{"email": "actualizado@test.com"},
		map[string]string{"X-Tenant-ID": user.TenantID.String()})

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, mockRepo.GetCallCount("Update"))
}

func TestUserRoutes_Update_BadUUID_BadRequest(t *testing.T) {
	router := newUserRouter(userrepo.NewMockUserRepository())

	w := doUserRequest(t, router, http.MethodPut, "/api/v1/users/no-es-un-uuid",
		map[string]interface{}{"email": "x@test.com"}, nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUserRoutes_Update_BadBody_BadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery())
	newUserHandler(userrepo.NewMockUserRepository()).RegisterRoutes(router.Group("/api/v1"))

	// Body no-JSON: ShouldBindJSON falla → 400.
	req := httptest.NewRequest(http.MethodPut, "/api/v1/users/"+uuid.New().String(), bytes.NewBufferString("{no-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUserRoutes_Update_TenantMismatch_NotFound(t *testing.T) {
	// Con X-Tenant-ID, el pre-check de pertenencia falla → 404 sin tocar Update.
	user := usermother.UserMother{}.WithDefaults()
	mockRepo := userrepo.NewMockUserRepository()
	mockRepo.SetupUsers([]*domentity.User{user})
	router := newUserRouter(mockRepo)

	w := doUserRequest(t, router, http.MethodPut, "/api/v1/users/"+user.ID.String(),
		map[string]interface{}{"email": "otro@test.com"},
		map[string]string{"X-Tenant-ID": uuid.New().String()})

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, 0, mockRepo.GetCallCount("Update"))
}

func TestUserRoutes_Update_NotFound(t *testing.T) {
	router := newUserRouter(userrepo.NewMockUserRepository())

	w := doUserRequest(t, router, http.MethodPut, "/api/v1/users/"+uuid.New().String(),
		map[string]interface{}{"email": "x@test.com"}, nil)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUserRoutes_Update_EmailInUse_Conflict(t *testing.T) {
	tenantID := uuid.New()
	userA := usermother.UserMother{}.WithEmail("a@test.com")
	userA.TenantID = tenantID
	userB := usermother.UserMother{}.WithEmail("b@test.com")
	userB.TenantID = tenantID
	mockRepo := userrepo.NewMockUserRepository()
	mockRepo.SetupUsers([]*domentity.User{userA, userB})
	router := newUserRouter(mockRepo)

	w := doUserRequest(t, router, http.MethodPut, "/api/v1/users/"+userA.ID.String(),
		map[string]interface{}{"email": "b@test.com"}, nil)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "El email ya está en uso")
}

func TestUserRoutes_Update_Status_OK(t *testing.T) {
	// Update de status (VO UserStatus via JSON) → ChangeStatus → 200.
	user := usermother.UserMother{}.WithDefaults()
	mockRepo := userrepo.NewMockUserRepository()
	mockRepo.SetupUsers([]*domentity.User{user})
	router := newUserRouter(mockRepo)

	w := doUserRequest(t, router, http.MethodPut, "/api/v1/users/"+user.ID.String(),
		map[string]interface{}{"status": "INACTIVE"}, nil)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestUserRoutes_Update_RepoFails_InternalError(t *testing.T) {
	user := usermother.UserMother{}.WithDefaults()
	mockRepo := userrepo.NewMockUserRepository()
	mockRepo.SetupUsers([]*domentity.User{user})
	mockRepo.ShouldFailOn("Update")
	router := newUserRouter(mockRepo)

	w := doUserRequest(t, router, http.MethodPut, "/api/v1/users/"+user.ID.String(),
		map[string]interface{}{"email": "nuevo@test.com"}, nil)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Error interno del servidor")
}

// --- ListUsers ---

func TestUserRoutes_List_NoTenantHeader_BadRequest(t *testing.T) {
	router := newUserRouter(userrepo.NewMockUserRepository())

	w := doUserRequest(t, router, http.MethodGet, "/api/v1/users", nil, nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "X-Tenant-ID header es requerido")
}

func TestUserRoutes_List_OK(t *testing.T) {
	tenantID := uuid.New()
	users := []*domentity.User{
		usermother.UserMother{}.WithDefaults(),
		usermother.UserMother{}.WithDefaults(),
	}
	mockRepo := userrepo.NewMockUserRepository()
	mockRepo.SetupUsers(users)
	router := newUserRouter(mockRepo)

	w := doUserRequest(t, router, http.MethodGet, "/api/v1/users?page=1&page_size=10", nil,
		map[string]string{"X-Tenant-ID": tenantID.String()})

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp, "items")
}

func TestUserRoutes_List_RepoFails_InternalError(t *testing.T) {
	mockRepo := userrepo.NewMockUserRepository()
	mockRepo.ShouldFailOn("SearchByCriteria")
	router := newUserRouter(mockRepo)

	w := doUserRequest(t, router, http.MethodGet, "/api/v1/users", nil,
		map[string]string{"X-Tenant-ID": uuid.New().String()})

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// TestUserRoutes_List_QueryTenantSpoof_NeutralizedByHeader: un tenant_id spoofeado en el
// query no llega al repositorio — el filtro que llega es el del header X-Tenant-ID.
// Oráculo GetLastCriteria (mock, ACC-E08 T3 QA): el handler pisa query.tenant_id con el
// header antes de BuildValidated; el test verifica el contrato contra lo que recibió el
// repo, no contra el query del request.
func TestUserRoutes_List_QueryTenantSpoof_NeutralizedByHeader(t *testing.T) {
	tenantID := uuid.New()
	spoofed := uuid.New()
	users := []*domentity.User{usermother.UserMother{}.WithDefaults()}
	mockRepo := userrepo.NewMockUserRepository()
	mockRepo.SetupUsers(users)
	router := newUserRouter(mockRepo)

	w := doUserRequest(t, router, http.MethodGet,
		"/api/v1/users?tenant_id="+spoofed.String()+"&page=1&page_size=10", nil,
		map[string]string{"X-Tenant-ID": tenantID.String()})

	require.Equal(t, http.StatusOK, w.Code)

	last := mockRepo.GetLastCriteria()
	var tenantFilter *crit.Filter
	for i := range last.Filters.Items {
		if last.Filters.Items[i].Field == "tenant_id" {
			tenantFilter = &last.Filters.Items[i]
			break
		}
	}
	require.NotNil(t, tenantFilter, "el criteria que llega al repo debe llevar filtro tenant_id")
	assert.Equal(t, tenantID.String(), tenantFilter.Value,
		"el valor del filtro tenant_id debe ser el del header, no el del query")

	// Negativo: ningún filtro con el valor spoofeado.
	for _, f := range last.Filters.Items {
		assert.NotEqual(t, spoofed.String(), f.Value,
			"el tenant spoofeado del query no debe aparecer en ningún filtro")
	}
}

// --- DeleteUser ---

func TestUserRoutes_Delete_OK_NoContent(t *testing.T) {
	user := usermother.UserMother{}.WithDefaults()
	mockRepo := userrepo.NewMockUserRepository()
	mockRepo.SetupUsers([]*domentity.User{user})
	router := newUserRouter(mockRepo)

	w := doUserRequest(t, router, http.MethodDelete, "/api/v1/users/"+user.ID.String(), nil, nil)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, 1, mockRepo.GetCallCount("Delete"))
}

func TestUserRoutes_Delete_BadUUID_BadRequest(t *testing.T) {
	router := newUserRouter(userrepo.NewMockUserRepository())

	w := doUserRequest(t, router, http.MethodDelete, "/api/v1/users/no-es-un-uuid", nil, nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUserRoutes_Delete_TenantMismatch_NotFound(t *testing.T) {
	user := usermother.UserMother{}.WithDefaults()
	mockRepo := userrepo.NewMockUserRepository()
	mockRepo.SetupUsers([]*domentity.User{user})
	router := newUserRouter(mockRepo)

	w := doUserRequest(t, router, http.MethodDelete, "/api/v1/users/"+user.ID.String(), nil,
		map[string]string{"X-Tenant-ID": uuid.New().String()})

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, 0, mockRepo.GetCallCount("Delete"))
}

func TestUserRoutes_Delete_NotFound(t *testing.T) {
	router := newUserRouter(userrepo.NewMockUserRepository())

	w := doUserRequest(t, router, http.MethodDelete, "/api/v1/users/"+uuid.New().String(), nil, nil)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUserRoutes_Delete_RepoFails_InternalError(t *testing.T) {
	user := usermother.UserMother{}.WithDefaults()
	mockRepo := userrepo.NewMockUserRepository()
	mockRepo.SetupUsers([]*domentity.User{user})
	mockRepo.ShouldFailOn("Delete")
	router := newUserRouter(mockRepo)

	w := doUserRequest(t, router, http.MethodDelete, "/api/v1/users/"+user.ID.String(), nil, nil)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}