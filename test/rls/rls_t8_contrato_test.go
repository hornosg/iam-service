//go:build integration

// Fase TEST de ACC-E02 T8 — completa el contrato del criterio de cierre
// ("un token del tenant A no puede leer NI ESCRIBIR filas del tenant B,
// verificado por HTTP contra el servicio corriendo") con dos familias de
// evidencia que rls_isolation_test.go no cubre:
//
//  1. CONTROL POSITIVO de PUT /users/:id con persistencia verificada. La
//     corrida EXEC exigió ese control para tenants ("un 404 universal —ruta
//     rota o fail-closed global— es indistinguible de aislamiento") pero el
//     PUT de users quedó sólo con el negativo.
//  2. SPOOFING DEL HEADER X-Tenant-ID. En /api/v1/users* el middleware
//     TenantValidation está en ExcludedRoutes (src/main.go) y el
//     UserCriteriaBuilder filtra el listado por el header, no por el JWT.
//     Si un token de A presenta X-Tenant-ID de B, el único control que
//     queda es RLS: el contrato exige evidencia de ese camino también.
//
// Oráculo de existencia: antes de cada negativo se verifica por SuperDB que
// la fila objetivo de B existe — sin eso, un 404 probaría un seed roto, no
// aislamiento (técnica State-Based: control de estado previo).
package rls_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRLS_ContratoT8_ControlesYHeaderSpoofing(t *testing.T) {
	ts := newTestRLSServer(t)
	base := ts.Server.URL + "/api/v1"

	// — Control de existencia del oráculo: las filas de B están en la base.
	// Sin este control, todos los 404 de abajo podrían significar que el
	// seed falló silenciosamente y el "aislamiento" sería un espejismo.
	t.Run("las filas objetivo de B existen en la base", func(t *testing.T) {
		var usersB int
		require.NoError(t, ts.SuperDB.QueryRow(
			`SELECT COUNT(*) FROM users WHERE id = $1 AND tenant_id = $2`,
			ts.UserB, ts.TenantB).Scan(&usersB))
		assert.Equal(t, 1, usersB, "el user de B debe existir para que el 404 signifique aislamiento")

		var tenantsB int
		require.NoError(t, ts.SuperDB.QueryRow(
			`SELECT COUNT(*) FROM tenants WHERE id = $1`, ts.TenantB).Scan(&tenantsB))
		assert.Equal(t, 1, tenantsB, "el tenant de B debe existir para que el 404 signifique aislamiento")
	})

	// — Control positivo de PUT /users/:id: la misma ruta que abajo se
	// prueba como negativo cross-tenant debe MUTAR de verdad para el propio
	// tenant. Un 200 sin persistencia no distingue nada (misma lección que
	// la corrida EXEC fijó para tenants).
	t.Run("control positivo: A actualiza su propio user y persiste", func(t *testing.T) {
		url := fmt.Sprintf("%s/users/%s", base, ts.UserA.String())
		body := map[string]interface{}{"email": "admin-a-renamed@example.com"}
		resp := putJSON(t, url, ts.TokenA, body, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var email string
		require.NoError(t, ts.SuperDB.QueryRow(
			`SELECT email FROM users WHERE id = $1`, ts.UserA).Scan(&email))
		assert.Equal(t, "admin-a-renamed@example.com", email,
			"el PUT propio debe mutar de verdad: sin persistencia, el 404 cross-tenant no prueba nada")
	})

	// — Spoofing del header: el token de A se presenta con el X-Tenant-ID
	// de B. El criteria builder arma `WHERE tenant_id = B` (header) y la
	// única barrera restante es la policy RLS evaluada con el tenant del
	// JWT. Ningún user de B puede aparecer en la respuesta.
	t.Run("A con X-Tenant-ID de B NO lista los users de B", func(t *testing.T) {
		url := base + "/users"
		resp := get(t, url, ts.TokenA, map[string]string{"X-Tenant-ID": ts.TenantB.String()})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode,
			"el spoofing del header debe resolverse como listado vacío, no como error 500")

		var listResp map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
		items, ok := listResp["items"].([]interface{})
		require.True(t, ok, "expected items array")
		emails := []string{}
		for _, it := range items {
			m, ok := it.(map[string]interface{})
			require.True(t, ok)
			emails = append(emails, m["email"].(string))
		}
		assert.NotContains(t, emails, "admin-b@example.com",
			"el header de otro tenant no puede derrotar a RLS y exponer los users de B")
	})

	// GET por ID con el header "legítimo" de la víctima: el chequeo a nivel
	// handler (user_handler.go, user.TenantID == header) PASARÍA, porque el
	// header coincide con el tenant real de la fila objetivo. Si este
	// request responde 404, el bloqueo es de RLS — no hay otra capa.
	t.Run("A con X-Tenant-ID de B NO lee el user de B por ID", func(t *testing.T) {
		url := fmt.Sprintf("%s/users/%s", base, ts.UserB.String())
		resp := get(t, url, ts.TokenA, map[string]string{"X-Tenant-ID": ts.TenantB.String()})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode,
			"con el header matching el tenant de la víctima, sólo RLS puede bloquear la lectura")
	})

	// Escritura con el mismo spoofing: PUT sobre el user de B con el header
	// de B. El 404 se ancla con no-mutación verificada por SuperDB.
	t.Run("A con X-Tenant-ID de B NO escribe el user de B", func(t *testing.T) {
		url := fmt.Sprintf("%s/users/%s", base, ts.UserB.String())
		body := map[string]interface{}{"email": "hackeado-por-a@example.com"}
		resp := putJSON(t, url, ts.TokenA, body, map[string]string{"X-Tenant-ID": ts.TenantB.String()})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		var email string
		require.NoError(t, ts.SuperDB.QueryRow(
			`SELECT email FROM users WHERE id = $1`, ts.UserB).Scan(&email))
		assert.Equal(t, "admin-b@example.com", email,
			"el PUT con header spoofeado no debe mutar la fila de B")
	})

	// — Control recíproco B→A (T8c: un solo sentido no es evidencia):
	// el mismo spoofing en la dirección opuesta.
	t.Run("B con X-Tenant-ID de A NO lista los users de A", func(t *testing.T) {
		url := base + "/users"
		resp := get(t, url, ts.TokenB, map[string]string{"X-Tenant-ID": ts.TenantA.String()})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode,
			"el spoofing del header debe resolverse como listado vacío, no como error 500")

		var listResp map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
		items, ok := listResp["items"].([]interface{})
		require.True(t, ok, "expected items array")
		emails := []string{}
		for _, it := range items {
			m, ok := it.(map[string]interface{})
			require.True(t, ok)
			emails = append(emails, m["email"].(string))
		}
		// El email vigente de A es el renombrado por el control positivo de
		// arriba; el oráculo apunta al dato actual, no al original.
		assert.NotContains(t, emails, "admin-a-renamed@example.com",
			"el header de otro tenant no puede derrotar a RLS y exponer los users de A")
	})
}