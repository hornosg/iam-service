//go:build integration

// ACC-E02 T8d — evidencia por HTTP contra el servicio corriendo (testcontainers,
// migraciones reales, roles account_app/iam_login NOBYPASSRLS) de que el GUC
// `app.is_system_admin` se deriva SÓLO de una autoridad verificada por Authorize
// y de que el branch cross-tenant de la policy de `tenants` funciona para quien
// la tiene.
//
// Condición vinculante del gate L4 de T8b (objeción (C)-1): derivar el GUC del
// claim crudo es una escalada cross-tenant directa — abre la tabla `tenants`
// entera. Los subtests de acá prueban las dos caras:
//   - el claim `is_system_admin` inyectado en un token tenant_admin NO abre nada;
//   - POST /tenants (provision, scope tenant:provision a secas) y el listado
//     cross-tenant de system:admin SÍ funcionan.
package rls_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeSystemAdminToken firma un JWT humano con rol system_admin (MapClaims
// directos: el middleware Authorize parsea claims genéricos, no el struct del
// adapter). Con tenant_id de A para ejercitar el path en que ambos GUCs
// (tenant + sysadmin) viajan juntos.
func makeSystemAdminToken(t *testing.T, ts *testRLSServer) string {
	t.Helper()
	claims := jwt.MapClaims{
		"user_id":   ts.UserA.String(),
		"tenant_id": ts.TenantA.String(),
		"roles":     []string{"system_admin"},
		"namespace": testNamespace,
		"exp":       time.Now().Add(15 * time.Minute).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	str, err := token.SignedString([]byte(testJWTSecret))
	require.NoError(t, err)
	return str
}

// makeForgedClaimToken firma un JWT tenant_admin al que se le INYECTA el claim
// `is_system_admin: true` — el ataque que la condición vinculante prohíbe abrir.
func makeForgedClaimToken(t *testing.T, ts *testRLSServer) string {
	t.Helper()
	claims := jwt.MapClaims{
		"user_id":         ts.UserA.String(),
		"tenant_id":       ts.TenantA.String(),
		"roles":           []string{"tenant_admin"},
		"is_system_admin": true, // claim crudo que ninguna ruta coteja
		"namespace":       testNamespace,
		"exp":             time.Now().Add(15 * time.Minute).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	str, err := token.SignedString([]byte(testJWTSecret))
	require.NoError(t, err)
	return str
}

func requestWithKey(t *testing.T, method, url, apiKey string, body interface{}) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, reader)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func parseBody(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	defer resp.Body.Close()
	var out map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return out
}

func TestRLS_SystemAdminGUC(t *testing.T) {
	ts := newTestRLSServer(t)
	base := ts.Server.URL + "/api/v1"

	t.Run("claim crudo is_system_admin NO abre cross-tenant", func(t *testing.T) {
		// El admin de A (rol tenant_admin, tenant A verificado) inyecta el claim
		// y apunta al tenant B. Sin autoridad system_admin verificada, la policy
		// de `tenants` no abre: 404, no 200.
		forged := makeForgedClaimToken(t, ts)
		resp := get(t, base+"/tenants/"+ts.TenantB.String(), forged, nil)
		defer resp.Body.Close()
		assert.Equal(t, 404, resp.StatusCode,
			"un claim is_system_admin inyectado no debe abrir el branch cross-tenant")

		// El mismo token SÍ ve lo suyo: el fallo no es global (un fail-closed
		// total podría esconder un GUC que no se fija en ninguna dirección).
		respOwn := get(t, base+"/tenants/"+ts.TenantA.String(), forged, nil)
		defer respOwn.Body.Close()
		assert.Equal(t, 200, respOwn.StatusCode,
			"el mismo token debe seguir viendo su propio tenant (positivo propio)")
	})

	t.Run("listado cross-tenant con scope S2S system:admin", func(t *testing.T) {
		resp := get(t, base+"/tenants", "", map[string]string{"X-API-Key": testSalesKey})
		defer resp.Body.Close()
		require.Equal(t, 200, resp.StatusCode)
		body := parseBody(t, resp)
		total := int(body["total_count"].(float64))
		assert.GreaterOrEqual(t, total, 2,
			"system:admin debe listar tenants cross-tenant (sembramos A y B)")
	})

	t.Run("listado cross-tenant con JWT system_admin", func(t *testing.T) {
		token := makeSystemAdminToken(t, ts)
		resp := get(t, base+"/tenants", token, nil)
		defer resp.Body.Close()
		require.Equal(t, 200, resp.StatusCode)
		body := parseBody(t, resp)
		total := int(body["total_count"].(float64))
		assert.GreaterOrEqual(t, total, 2,
			"el rol system_admin verificado por el gate debe abrir el branch cross-tenant")
	})

	t.Run("POST /tenants con scope tenant:provision (sin system:admin) crea el tenant", func(t *testing.T) {
		// whatsapp-agent tiene tenant:provision a secas. El INSERT se materializa
		// por la rama id = app.tenant_id del WITH CHECK (autoridad mínima),
		// NUNCA por app.is_system_admin (condición vinculante del gate de T8b).
		slug := "t8d-provisioned"
		owner := uuid.New()
		resp := requestWithKey(t, http.MethodPost, base+"/tenants", testWhatsappKey, map[string]interface{}{
			"name":        "Tenant Provisionado T8d",
			"slug":        slug,
			"description": "tenant creado por T8d vía scope tenant:provision",
			"type":        "BUSINESS",
			"owner_id":    owner.String(),
		})
		defer resp.Body.Close()
		require.Equal(t, 201, resp.StatusCode, "body: %s", func() string {
			b, _ := io.ReadAll(resp.Body)
			return string(b)
		}())

		var count int
		require.NoError(t, ts.SuperDB.QueryRow(
			`SELECT COUNT(*) FROM tenants WHERE slug = $1`, slug).Scan(&count))
		assert.Equal(t, 1, count, "el tenant provisionado debe existir en la base")
	})

	t.Run("el caller de provision NO gana lectura cross-tenant", func(t *testing.T) {
		// Sin system:admin, la key de provision no pasa el gate tenant-scoped y,
		// aunque pasara, no tendría GUC: fail-closed.
		resp := get(t, base+"/tenants/"+ts.TenantA.String(), "", map[string]string{"X-API-Key": testWhatsappKey})
		defer resp.Body.Close()
		assert.NotEqual(t, 200, resp.StatusCode,
			"tenant:provision no debe leer tenants existentes")
	})

	t.Run("un caller sin ninguna autoridad sigue fail-closed", func(t *testing.T) {
		// admin de A (tenant_admin) NO debe ver el tenant B aunque la policy
		// tenga un branch abierto — ese branch exige el GUC verificado.
		resp := get(t, base+"/tenants/"+ts.TenantB.String(), ts.TokenA, nil)
		defer resp.Body.Close()
		assert.Equal(t, 404, resp.StatusCode)
	})
}
