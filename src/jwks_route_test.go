package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/hornosg/iam-service/src/identity/infrastructure/config"
)

// ACC-E03 T7 — contrato del JWKS verificado sobre el router REAL (buildRouter),
// no sobre un engine armado a mano: el handler aislado no prueba que la ruta
// esté fuera de TenantValidation, del gate de revocación ni de authorize.
//
// Contrato (épica ACC-E03 T7): endpoint público (sin auth) que devuelve el/los
// JWK público(s) activo(s) con su kid; nunca expone la privada.

const jwksTestSecret = "golden-test-secret-not-a-real-credential-0123456789"

// jwksRouter monta el router real con una clave de firma inline (dev) y
// devuelve la pública esperada.
func jwksRouter(t *testing.T) (*gin.Engine, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	t.Setenv("JWT_PRIVATE_KEY_FILE", "")
	t.Setenv("JWT_PRIVATE_KEY", string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})))
	t.Setenv("JWT_KEY_ID", "")
	t.Setenv("GIN_MODE", "debug")
	t.Setenv("JWT_SECRET", jwksTestSecret)
	t.Setenv("PROMETHEUS_ENABLED", "true")
	t.Setenv(switchEnvKey, "")
	gin.SetMode(gin.TestMode)
	return buildRouterForTest(t), key
}

func serve(r *gin.Engine, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

// Criterio (a) in-process: sin Authorization ni X-Tenant-ID, el router real
// responde 200 con {"keys":[...]} y keys[0] lleva kid + kty, y el kid es el
// de la clave de firma cargada (el de T2: thumbprint del material montado).
func TestJWKSRoute_PublicaSinAuthConElKidDeLaClaveDeFirma(t *testing.T) {
	r, key := jwksRouter(t)

	w := serve(r, http.MethodGet, "/.well-known/jwks.json")

	if w.Code != http.StatusOK {
		t.Fatalf("GET /.well-known/jwks.json sin auth → %d, se esperaba 200 (el JWKS es público): %s", w.Code, w.Body.String())
	}
	var doc struct {
		Keys []map[string]any `json:"keys"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("el cuerpo no es un JWKS RFC 7517 en la raíz (¿envelope?): %v — %s", err, w.Body.String())
	}
	if len(doc.Keys) != 1 {
		t.Fatalf("se esperaba exactamente la clave activa, hay %d: %s", len(doc.Keys), w.Body.String())
	}
	if doc.Keys[0]["kty"] != "RSA" {
		t.Errorf("kty = %v, se esperaba RSA", doc.Keys[0]["kty"])
	}
	wantKid := config.SigningKeyKID(&key.PublicKey)
	if doc.Keys[0]["kid"] != wantKid {
		t.Errorf("kid publicado = %v, el de la clave de firma es %s", doc.Keys[0]["kid"], wantKid)
	}
	if n, _ := doc.Keys[0]["n"].(string); n != base64.RawURLEncoding.EncodeToString(key.N.Bytes()) {
		t.Errorf("n publicado no es el módulo de la clave de firma")
	}
}

// Control del control: en el MISMO router, una ruta protegida sin token NO da
// 200. Si diera 200, el 200 del JWKS no probaría que la ruta es pública a
// propósito — probaría que la auth está apagada en el router de test.
func TestJWKSRoute_ControlUnaRutaProtegidaSinTokenNoPasa(t *testing.T) {
	r, _ := jwksRouter(t)

	w := serve(r, http.MethodGet, "/api/v1/roles")

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/v1/roles sin token → %d, se esperaba 401 — sin este control el test de JWKS público no discrimina", w.Code)
	}
}

// PRUEBA NEGATIVA de fuga (criterio c) sobre el router real: el cuerpo no lleva
// miembros privados de un JWK RSA/EC (d, p, q, dp, dq, qi), ni un JWK simétrico
// (kty "oct" con "k"), ni PEM privado, ni el exponente privado en base64url,
// ni el secreto simétrico compartido (JWT_SECRET) que todavía está en el env.
func TestJWKSRoute_NoExponeMaterialPrivadoNiSecretoSimetrico(t *testing.T) {
	r, key := jwksRouter(t)

	raw := serve(r, http.MethodGet, "/.well-known/jwks.json").Body.String()

	leak := regexp.MustCompile(`(?i)"(d|p|q|dp|dq|qi|k)"\s*:|PRIVATE|"oct"`)
	if m := leak.FindString(raw); m != "" {
		t.Errorf("el JWKS expone material privado/simétrico (%q): %s", m, raw)
	}
	for nombre, secreto := range map[string]string{
		"d (exponente privado)": base64.RawURLEncoding.EncodeToString(key.D.Bytes()),
		"p (primo)":             base64.RawURLEncoding.EncodeToString(key.Primes[0].Bytes()),
		"JWT_SECRET":            jwksTestSecret,
		"JWT_SECRET base64url":  base64.RawURLEncoding.EncodeToString([]byte(jwksTestSecret)),
	} {
		if strings.Contains(raw, secreto) {
			t.Errorf("el JWKS contiene %s", nombre)
		}
	}
}

// El JWKS es de sólo lectura: ningún método de escritura sobre la ruta llega
// a un handler que responda 200 (no se puede "publicar" una clave por HTTP).
func TestJWKSRoute_SoloGET(t *testing.T) {
	r, _ := jwksRouter(t)

	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		if w := serve(r, m, "/.well-known/jwks.json"); w.Code == http.StatusOK {
			t.Errorf("%s /.well-known/jwks.json → 200; la ruta debe ser GET-only", m)
		}
	}
}
