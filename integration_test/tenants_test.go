//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Helpers de tipos para aserciones ---

type tenantResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	IsActive bool   `json:"is_active"`
}

type listTenantsResponse struct {
	Tenants    []tenantResponse `json:"tenants"`
	TotalCount int              `json:"total_count"`
	Page       int              `json:"page"`
	PageSize   int              `json:"page_size"`
	TotalPages int              `json:"total_pages"`
}

func buildCreateTenantBody(name, slug string) map[string]interface{} {
	return map[string]interface{}{
		"name":        name,
		"slug":        slug,
		"description": "Tenant de prueba de integración",
		"type":        "STARTUP",
		"owner_id":    uuid.New().String(),
	}
}

func postJSON(t *testing.T, url string, body interface{}) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	return resp
}

func putJSON(t *testing.T, url string, body interface{}) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(b))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func deleteRequest(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	defer resp.Body.Close()
	require.NoError(t, json.NewDecoder(resp.Body).Decode(v))
}

// --- Tests de Tenants ---

func TestTenants_POST_HappyPath_Returns201(t *testing.T) {
	srv := newTestServer(t)
	url := baseURL(srv) + "/tenants"

	resp := postJSON(t, url, buildCreateTenantBody("Tienda Alpha", "tienda-alpha"))
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var tenant tenantResponse
	decodeJSON(t, resp, &tenant)
	assert.Equal(t, "Tienda Alpha", tenant.Name)
	assert.Equal(t, "tienda-alpha", tenant.Slug)
	assert.Equal(t, "STARTUP", tenant.Type)
	assert.Equal(t, "ACTIVE", tenant.Status)
	assert.True(t, tenant.IsActive)
	assert.NotEmpty(t, tenant.ID)
	_, err := uuid.Parse(tenant.ID)
	assert.NoError(t, err, "id debe ser un UUID válido")
}

func TestTenants_POST_DuplicateSlug_Returns409(t *testing.T) {
	srv := newTestServer(t)
	url := baseURL(srv) + "/tenants"

	postJSON(t, url, buildCreateTenantBody("Primero", "slug-unico"))
	resp := postJSON(t, url, buildCreateTenantBody("Segundo", "slug-unico"))
	defer resp.Body.Close()

	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestTenants_POST_MissingRequiredFields_Returns400(t *testing.T) {
	srv := newTestServer(t)
	url := baseURL(srv) + "/tenants"

	resp := postJSON(t, url, map[string]interface{}{"name": "Solo nombre"})
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestTenants_GET_ByID_HappyPath_Returns200(t *testing.T) {
	srv := newTestServer(t)
	base := baseURL(srv)

	// Crear
	createResp := postJSON(t, base+"/tenants", buildCreateTenantBody("Tienda Beta", "tienda-beta"))
	var created tenantResponse
	decodeJSON(t, createResp, &created)
	require.NotEmpty(t, created.ID)

	// Obtener por ID
	resp := getRequest(t, fmt.Sprintf("%s/tenants/%s", base, created.ID))
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var fetched tenantResponse
	decodeJSON(t, resp, &fetched)
	assert.Equal(t, created.ID, fetched.ID)
	assert.Equal(t, "Tienda Beta", fetched.Name)
}

func TestTenants_GET_ByID_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	url := fmt.Sprintf("%s/tenants/%s", baseURL(srv), uuid.New().String())

	resp := getRequest(t, url)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestTenants_GET_ByID_InvalidUUID_Returns400(t *testing.T) {
	srv := newTestServer(t)
	url := baseURL(srv) + "/tenants/not-a-uuid"

	resp := getRequest(t, url)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestTenants_GET_List_ReturnsPaginationShape(t *testing.T) {
	srv := newTestServer(t)
	base := baseURL(srv)

	// Crear dos tenants
	postJSON(t, base+"/tenants", buildCreateTenantBody("Tenant List 1", "tenant-list-1"))
	postJSON(t, base+"/tenants", buildCreateTenantBody("Tenant List 2", "tenant-list-2"))

	resp := s2sGet(t, base+"/tenants?page=1&page_size=10", keySystemAdminSvc)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var listResp listTenantsResponse
	decodeJSON(t, resp, &listResp)

	assert.GreaterOrEqual(t, listResp.TotalCount, 2)
	assert.Equal(t, 1, listResp.Page)
	assert.Equal(t, 10, listResp.PageSize)
	assert.Greater(t, listResp.TotalPages, 0)
	assert.GreaterOrEqual(t, len(listResp.Tenants), 2)
}

func TestTenants_GET_List_DefaultPagination_Returns200(t *testing.T) {
	srv := newTestServer(t)
	base := baseURL(srv)

	resp := s2sGet(t, base+"/tenants", keySystemAdminSvc)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var listResp listTenantsResponse
	decodeJSON(t, resp, &listResp)
	// Verificar que los campos de paginación están presentes
	assert.GreaterOrEqual(t, listResp.TotalCount, 0)
	assert.Greater(t, listResp.PageSize, 0)
}

func TestTenants_PUT_Update_HappyPath_Returns200(t *testing.T) {
	srv := newTestServer(t)
	base := baseURL(srv)

	createResp := postJSON(t, base+"/tenants", buildCreateTenantBody("Tienda Original", "tienda-original"))
	var created tenantResponse
	decodeJSON(t, createResp, &created)
	require.NotEmpty(t, created.ID)

	newName := "Tienda Actualizada"
	updateBody := map[string]interface{}{
		"name":        newName,
		"description": "Tienda Original Actualizada",
	}
	resp := putJSON(t, fmt.Sprintf("%s/tenants/%s", base, created.ID), updateBody)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var updated tenantResponse
	decodeJSON(t, resp, &updated)
	assert.Equal(t, newName, updated.Name)
	assert.Equal(t, created.ID, updated.ID)
}

func TestTenants_PUT_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	url := fmt.Sprintf("%s/tenants/%s", baseURL(srv), uuid.New().String())

	resp := putJSON(t, url, map[string]interface{}{"name": "No existe"})
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestTenants_DELETE_HappyPath_Returns204(t *testing.T) {
	srv := newTestServer(t)
	base := baseURL(srv)

	createResp := postJSON(t, base+"/tenants", buildCreateTenantBody("Tienda Borrar", "tienda-borrar"))
	var created tenantResponse
	decodeJSON(t, createResp, &created)
	require.NotEmpty(t, created.ID)

	resp := deleteRequest(t, fmt.Sprintf("%s/tenants/%s", base, created.ID))
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestTenants_DELETE_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	url := fmt.Sprintf("%s/tenants/%s", baseURL(srv), uuid.New().String())

	resp := deleteRequest(t, url)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
