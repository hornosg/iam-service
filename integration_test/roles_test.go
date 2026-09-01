//go:build integration

package integration_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Tipos de respuesta para Roles ---

type roleResponse struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	IsActive    bool     `json:"is_active"`
	IsSystem    bool     `json:"is_system"`
	IsTenant    bool     `json:"is_tenant"`
	Permissions []string `json:"permissions"`
}

type listRolesResponse struct {
	Items      []roleResponse `json:"items"`
	TotalCount int            `json:"total_count"`
	Page       int            `json:"page"`
	PageSize   int            `json:"page_size"`
	TotalPages int            `json:"total_pages"`
}

func buildCreateRoleBody(name string) map[string]interface{} {
	return map[string]interface{}{
		"name":        name,
		"description": "Rol de prueba de integración para el sistema",
		"type":        "CUSTOM",
		"permissions": []string{"tenant:read", "user:read"},
	}
}

// buildCreateTenantRoleBody ya NO manda tenant_id (ACC-E02 T11 — objeción D5 de
// T10): `roles` es catálogo global y CreateRoleRequest no acepta tenant_id. Se
// conserva el nombre para trazar el criterio de cierre de la tarea.
func buildCreateTenantRoleBody(name string) map[string]interface{} {
	return map[string]interface{}{
		"name":        name,
		"description": "Rol de prueba de integración para tenant",
		"type":        "CUSTOM",
		"permissions": []string{"tenant:read", "user:read"},
	}
}

func s2sDelete(t *testing.T, url, key string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	require.NoError(t, err)
	if key != "" {
		req.Header.Set("X-API-Key", key)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func getRequest(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// --- Tests de Roles ---
//
// ACC-E02 T11: los endpoints de roles se ejercitan con los grupos reales
// (lectura → tenantScopedGroup, escritura → adminGroup), así que los requests
// llevan API keys S2S: `tenant:admin` para lectura, `system:admin` para
// escritura.

func TestRoles_POST_HappyPath_Returns201(t *testing.T) {
	srv := newTestServer(t)
	url := baseURL(srv) + "/roles"

	resp := s2sPostJSON(t, url, keySystemAdminSvc, buildCreateRoleBody("Rol Integración Alpha"))
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var role roleResponse
	decodeJSON(t, resp, &role)
	assert.Equal(t, "Rol Integración Alpha", role.Name)
	assert.Equal(t, "CUSTOM", role.Type)
	assert.True(t, role.IsActive)
	assert.False(t, role.IsSystem)
	assert.True(t, role.IsTenant)
	assert.NotEmpty(t, role.ID)
	_, err := uuid.Parse(role.ID)
	assert.NoError(t, err, "id debe ser UUID válido")
}

func TestRoles_POST_TenantAdminKey_Returns403(t *testing.T) {
	// Gate de escritura de T10: `tenant:admin` NO puede crear roles. Verificado
	// por HTTP contra el router con los grupos reales.
	srv := newTestServer(t)
	url := baseURL(srv) + "/roles"

	resp := s2sPostJSON(t, url, keyTenantAdminSvc, buildCreateTenantRoleBody("Rol Inyectado Por Tenant Admin"))
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestRoles_POST_WithoutKey_Returns401(t *testing.T) {
	srv := newTestServer(t)
	url := baseURL(srv) + "/roles"

	resp := postJSON(t, url, buildCreateRoleBody("Rol Sin Key"))
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestRoles_POST_DuplicateName_Returns409(t *testing.T) {
	srv := newTestServer(t)
	url := baseURL(srv) + "/roles"

	s2sPostJSON(t, url, keySystemAdminSvc, buildCreateRoleBody("Rol Duplicado"))
	resp := s2sPostJSON(t, url, keySystemAdminSvc, buildCreateRoleBody("Rol Duplicado"))
	defer resp.Body.Close()

	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestRoles_POST_InvalidType_Returns400(t *testing.T) {
	srv := newTestServer(t)
	url := baseURL(srv) + "/roles"

	body := map[string]interface{}{
		"name":        "Rol Tipo Invalido",
		"description": "Descripcion del rol de tipo invalido",
		"type":        "INVALID_TYPE",
	}
	resp := s2sPostJSON(t, url, keySystemAdminSvc, body)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRoles_POST_MissingRequiredFields_Returns400(t *testing.T) {
	srv := newTestServer(t)
	url := baseURL(srv) + "/roles"

	resp := s2sPostJSON(t, url, keySystemAdminSvc, map[string]interface{}{"name": "Solo nombre"})
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRoles_GET_ByID_HappyPath_Returns200(t *testing.T) {
	srv := newTestServer(t)
	base := baseURL(srv)

	createResp := s2sPostJSON(t, base+"/roles", keySystemAdminSvc, buildCreateRoleBody("Rol Get Test"))
	var created roleResponse
	decodeJSON(t, createResp, &created)
	require.NotEmpty(t, created.ID)

	// La lectura de /roles es accesible con scope tenant:admin (tenantScopedGroup).
	resp := s2sGet(t, fmt.Sprintf("%s/roles/%s", base, created.ID), keyTenantAdminSvc)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var fetched roleResponse
	decodeJSON(t, resp, &fetched)
	assert.Equal(t, created.ID, fetched.ID)
	assert.Equal(t, "Rol Get Test", fetched.Name)
}

func TestRoles_GET_ByID_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	url := fmt.Sprintf("%s/roles/%s", baseURL(srv), uuid.New().String())

	resp := s2sGet(t, url, keyTenantAdminSvc)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestRoles_GET_ByID_InvalidUUID_Returns400(t *testing.T) {
	srv := newTestServer(t)
	url := baseURL(srv) + "/roles/not-a-uuid"

	resp := s2sGet(t, url, keyTenantAdminSvc)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRoles_GET_List_ReturnsPaginationShape(t *testing.T) {
	srv := newTestServer(t)
	base := baseURL(srv)

	s2sPostJSON(t, base+"/roles", keySystemAdminSvc, buildCreateRoleBody("Rol Lista 1"))
	s2sPostJSON(t, base+"/roles", keySystemAdminSvc, buildCreateRoleBody("Rol Lista 2"))

	resp := s2sGet(t, base+"/roles?page=1&page_size=10", keyTenantAdminSvc)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var listResp listRolesResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))

	// La migración de seed inserta 4 roles de sistema, más los 2 recién creados
	assert.GreaterOrEqual(t, listResp.TotalCount, 2)
	assert.Equal(t, 1, listResp.Page)
	assert.Equal(t, 10, listResp.PageSize)
	assert.Greater(t, listResp.TotalPages, 0)
	assert.NotEmpty(t, listResp.Items)
}

func TestRoles_PUT_Update_HappyPath_Returns200(t *testing.T) {
	srv := newTestServer(t)
	base := baseURL(srv)

	createResp := s2sPostJSON(t, base+"/roles", keySystemAdminSvc, buildCreateRoleBody("Rol Original"))
	var created roleResponse
	decodeJSON(t, createResp, &created)
	require.NotEmpty(t, created.ID)

	newDesc := "Descripcion actualizada del rol de prueba"
	updateBody := map[string]interface{}{
		"name":        "Rol Original",
		"description": newDesc,
	}
	resp := s2sPutJSON(t, fmt.Sprintf("%s/roles/%s", base, created.ID), keySystemAdminSvc, updateBody)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var updated roleResponse
	decodeJSON(t, resp, &updated)
	assert.Equal(t, newDesc, updated.Description)
	assert.Equal(t, created.ID, updated.ID)
}

func TestRoles_PUT_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	url := fmt.Sprintf("%s/roles/%s", baseURL(srv), uuid.New().String())

	resp := s2sPutJSON(t, url, keySystemAdminSvc, map[string]interface{}{"description": "No existe"})
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestRoles_DELETE_HappyPath_Returns204(t *testing.T) {
	srv := newTestServer(t)
	base := baseURL(srv)

	createResp := s2sPostJSON(t, base+"/roles", keySystemAdminSvc, buildCreateRoleBody("Rol A Eliminar"))
	var created roleResponse
	decodeJSON(t, createResp, &created)
	require.NotEmpty(t, created.ID)

	resp := s2sDelete(t, fmt.Sprintf("%s/roles/%s", base, created.ID), keySystemAdminSvc)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestRoles_DELETE_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	url := fmt.Sprintf("%s/roles/%s", baseURL(srv), uuid.New().String())

	resp := s2sDelete(t, url, keySystemAdminSvc)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestRoles_DELETE_SystemRole_Returns403(t *testing.T) {
	srv := newTestServer(t)
	base := baseURL(srv)

	// El seed crea roles de sistema — obtener su ID via listing
	listResp := s2sGet(t, base+"/roles?page=1&page_size=50", keyTenantAdminSvc)
	defer listResp.Body.Close()

	var list listRolesResponse
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&list))

	var systemRoleID string
	for _, r := range list.Items {
		if r.Type == "SYSTEM_ADMIN" {
			systemRoleID = r.ID
			break
		}
	}
	require.NotEmpty(t, systemRoleID, "debe existir al menos un rol de sistema del seed")

	resp := s2sDelete(t, fmt.Sprintf("%s/roles/%s", base, systemRoleID), keySystemAdminSvc)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}
