//go:build integration

// Smoke de cierre de ACC-E02 (T9): el flujo end-to-end POST /auth/login →
// token → endpoint post-auth del propio tenant, con el lookup de credencial
// corriendo bajo el rol acotado iam_login (pool de login) y todo lo demás bajo
// account_app con RLS.
//
// Criterio de cierre (ACC-E02 T9): "el login sigue funcionando con el rol
// acotado y el aislamiento de T8 pasa, EN LA MISMA CORRIDA". La misma corrida
// es la invocación de go test de este paquete: este smoke corre junto a
// TestRLS_CrossTenantIsolation y el resto de test/rls (aislamiento de T8/T8c)
// sobre migraciones reales 001→023, router real y ambos pools verificados
// NOBYPASSRLS por assertNoRLSBypass.
//
// Este smoke fue el que detectó que el login estaba roto: GetByEmail
// seleccionaba created_at/updated_at — columnas fuera del grant de la 017 — y
// el login con credenciales válidas devolvía 401 (permission denied tragado
// como credenciales inválidas). T5 no lo vio porque su verificación en vivo
// usó password inválida, que responde 401 tanto si el lookup funciona como si
// falla: el positivo acá es el que distingue.
//
// Para correr (misma corrida que el aislamiento de T8):
//   cd platform/iam-service && GOWORK=off go test -tags=integration ./test/rls/... -count=1 -v
package rls_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type smokeLoginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	User         struct {
		ID       string `json:"id"`
		Email    string `json:"email"`
		TenantID string `json:"tenant_id"`
		Status   string `json:"status"`
	} `json:"user"`
}

func TestRLS_SmokeDeCierre(t *testing.T) {
	ts := newTestRLSServer(t)
	base := ts.Server.URL + "/api/v1"

	// Flujo real: login por HTTP con la credencial sembrada (bcrypt de "123456"),
	// sin firmar tokens a mano. El lookup corre bajo iam_login.
	resp := postJSON(t, base+"/auth/login", "", map[string]string{
		"email":    "admin-a@example.com",
		"password": "123456",
		"provider": "LOCAL",
	})
	defer resp.Body.Close()

	var login smokeLoginResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&login), "el login debe devolver JSON decodable")

	t.Run("login con credenciales válidas → 200 bajo el rol acotado iam_login", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, resp.StatusCode, "el login debe funcionar con los grants acotados de la 017 (8 columnas de credencial)")
		assert.Equal(t, "Bearer", login.TokenType)
		assert.NotEmpty(t, login.AccessToken)
		assert.NotEmpty(t, login.RefreshToken)
		assert.Equal(t, "admin-a@example.com", login.User.Email)
		assert.Equal(t, ts.UserA.String(), login.User.ID)
		assert.Equal(t, ts.TenantA.String(), login.User.TenantID)
		assert.Equal(t, "ACTIVE", login.User.Status)
	})

	t.Run("login con password inválida → 401 (el positivo no es 'acepta todo')", func(t *testing.T) {
		bad := postJSON(t, base+"/auth/login", "", map[string]string{
			"email":    "admin-a@example.com",
			"password": "wrong-password",
			"provider": "LOCAL",
		})
		defer bad.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, bad.StatusCode)
	})

	require.NotEmpty(t, login.AccessToken, "sin access token no hay smoke: el login falló")

	t.Run("el token del login autoriza el endpoint post-auth del propio tenant (GET /users)", func(t *testing.T) {
		resp := get(t, base+"/users", login.AccessToken, map[string]string{"X-Tenant-ID": ts.TenantA.String()})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

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
		assert.Contains(t, emails, "admin-a@example.com")
		assert.NotContains(t, emails, "admin-b@example.com")
	})

	t.Run("el token del login lee su propio tenant (GET /tenants/:id)", func(t *testing.T) {
		resp := get(t, base+"/tenants/"+ts.TenantA.String(), login.AccessToken, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("el token del login NO ve el tenant B (aislamiento en el mismo flujo)", func(t *testing.T) {
		resp := get(t, base+"/tenants/"+ts.TenantB.String(), login.AccessToken, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("NEGATIVO: el endpoint post-auth rechaza sin access token (401, no abierto)", func(t *testing.T) {
		resp := get(t, base+"/users", "", nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"si GET /users responde otra cosa sin token, el middleware de autorización no está aplicando — el smoke no puede dar verde")
	})

	t.Run("NEGATIVO: login con email inexistente → 401, no 500 ni leak", func(t *testing.T) {
		resp := postJSON(t, base+"/auth/login", "", map[string]string{
			"email":    "no-existe@example.com",
			"password": "123456",
			"provider": "LOCAL",
		})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"un lookup vacío es credenciales inválidas (401), no un error del servidor; 500 indicaría que el escape pre-auth de la policy falló cerrado con error en vez de con 0 filas")
	})

	// NEGATIVO estructural del "con el rol acotado" del criterio de T9: el
	// positivo del login no distingue correr bajo iam_login de correr bajo un
	// pool más ancho. Si alguien "arregla" un login roto agrandando el grant de
	// la 017 o apuntando el lookup al pool de app, estos asserts fallan y
	// obligan a pasar por el gate L4 que firmó el corte de columnas (T1/T2).
	t.Run("NEGATIVO: el pool de login ES iam_login y el grant acota de verdad", func(t *testing.T) {
		var currentUser string
		require.NoError(t, ts.LoginDB.QueryRow("SELECT current_user").Scan(&currentUser))
		assert.Equal(t, "iam_login", currentUser,
			"el pool de login debe conectar como iam_login; si es otro rol, el smoke no está probando el rol acotado")

		// created_at/updated_at están fuera del grant por columnas de la 017 por
		// diseño (T1: "el path de credencial no las lee"). Leerlas debe dar
		// permission denied — es exactamente el bug que este smoke detectó.
		_, err := ts.LoginDB.Query("SELECT created_at FROM users LIMIT 1")
		require.Error(t, err, "iam_login NO puede leer created_at: el grant de la 017 es por columnas")
		assert.Contains(t, err.Error(), "permission denied")

		// T1-D1: prohibido GRANT UPDATE a iam_login. Un UPDATE de federated_id
		// con ese rol sería una primitiva de account-takeover cross-tenant.
		_, err = ts.LoginDB.Exec("UPDATE users SET federated_id = 'x' WHERE email = 'admin-a@example.com'")
		require.Error(t, err, "iam_login NO puede escribir users (T1-D1): un UPDATE con ese rol es takeover cross-tenant")
		assert.Contains(t, err.Error(), "permission denied")
	})
}