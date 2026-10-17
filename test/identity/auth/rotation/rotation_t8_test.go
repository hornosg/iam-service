// ACC-E03 T8 — simulacro de rotación de clave con solapamiento (ADR-003 §c:
// publicar → firmar → gracia → retirar), verificación IN-PROCESS (§c: el borde
// no tiene dual en rotación, T8(a) quedó re-alcanceado a in-process).
//
// Cada fase arma el verificador como lo arma router.go: la clave de firma +
// el keyring cargado desde JWT_PUBLIC_KEY_DIR por la función REAL de config
// (condición C1 de T7), y UNA instancia compartida por Parse, token_revocation,
// authorize (vía PublicKeyFor) y el JWKS (C2: lo publicado es lo aceptado).
// Rotar es cambiar los archivos del keyring y recrear: cada fase es un
// "recreate" con un directorio nuevo.
//
// Criterios cubiertos:
//   - (a) en gracia, token kid1 → 200 y token kid2 → 200 en los tres gates.
//   - (b) PRUEBA NEGATIVA: tras retirar kid1, kid2 → 200 y kid1 → 401. Si kid1
//     siguiera validando, la rotación es cosmética y el test falla.
//   - C4 de T7: el mismo directorio, leído por el verificador de flota de
//     go-shared (jwtkeyring), también deja de aceptar kid1 tras retirarlo.
package rotation_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/hornosg/go-shared/infrastructure/jwtkeyring"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authmw "github.com/hornosg/iam-service/src/access/infrastructure/middleware"
	"github.com/hornosg/iam-service/src/access/infrastructure/s2s"
	"github.com/hornosg/iam-service/src/identity/domain/value_object"
	"github.com/hornosg/iam-service/src/identity/infrastructure/adapter"
	"github.com/hornosg/iam-service/src/identity/infrastructure/config"
	identityctl "github.com/hornosg/iam-service/src/identity/infrastructure/controller"
	"github.com/hornosg/iam-service/src/identity/infrastructure/middleware"
	repo "github.com/hornosg/iam-service/test/identity/auth/infrastructure/persistence/repository"
)

const namespace = "mc"

// Fixtures de test (no secretos reales): dos pares RSA generados acá.
var (
	key1 = mustKey()
	key2 = mustKey()
)

func mustKey() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
}

func signingKey(k *rsa.PrivateKey) *adapter.SigningKey {
	return &adapter.SigningKey{PrivateKey: k, KID: config.SigningKeyKID(&k.PublicKey)}
}

// keyringDir escribe un keyring de flota: un "PUBLIC KEY" PKIX por clave.
func keyringDir(t *testing.T, keys ...*rsa.PrivateKey) string {
	t.Helper()
	dir := t.TempDir()
	for _, k := range keys {
		der, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
		require.NoError(t, err)
		pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
		name := config.SigningKeyKID(&k.PublicKey) + ".pem"
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), pemBytes, 0o644))
	}
	return dir
}

// deployment es una "instancia recreada" de iam: el verificador cargado por
// config desde el keyring + la clave de firma, montado en un router con JWKS,
// token_revocation y authorize — la misma composición que router.go.
type deployment struct {
	verifier *adapter.JWTServiceAdapter
	engine   *gin.Engine
}

func deploy(t *testing.T, signer *rsa.PrivateKey, dir string) deployment {
	t.Helper()
	t.Setenv("JWT_PUBLIC_KEY_DIR", dir)
	ring, err := config.LoadVerificationKeyringFromEnv()
	require.NoError(t, err)
	verifier := adapter.NewJWTServiceAdapterWithKeyring(signingKey(signer), ring)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET(identityctl.JWKSPath, identityctl.JWKSHandler(verifier))
	api := r.Group("/api/v1")
	api.Use(middleware.TokenRevocationCheck(middleware.TokenRevocationConfig{
		JWT:      verifier,
		AuthRepo: repo.NewMockAuthRepository(),
	}))
	// /me: sólo el gate de revocación — 200 si el token quedó autenticado.
	api.GET("/me", func(c *gin.Context) {
		if _, ok := c.Get("token_claims"); !ok {
			c.Status(http.StatusUnauthorized)
			return
		}
		c.Status(http.StatusOK)
	})
	registry := s2s.LoadFromEnvForTests(map[string]string{})
	factory := authmw.NewScopeMiddlewareFactory(namespace, registry, verifier)
	api.Group("", factory.RequireScopes(
		[]s2s.Scope{s2s.ScopeSystemAdmin, s2s.ScopeTenantAdmin}, "tenant_admin", "system_admin")).
		GET("/users", func(c *gin.Context) { c.Status(http.StatusOK) })
	return deployment{verifier: verifier, engine: r}
}

func (d deployment) login(t *testing.T) string {
	t.Helper()
	tok, err := d.verifier.Sign(&value_object.TokenClaims{
		JTI:       uuid.New(),
		Namespace: namespace,
		UserID:    uuid.New(),
		TenantID:  uuid.New(),
		Roles:     []string{"tenant_admin"},
		ExpiresAt: time.Now().Add(15 * time.Minute).Unix(),
	})
	require.NoError(t, err)
	return tok
}

func (d deployment) get(t *testing.T, path, token string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	d.engine.ServeHTTP(w, req)
	return w.Code
}

func (d deployment) jwksKIDs(t *testing.T) []string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, identityctl.JWKSPath, nil)
	w := httptest.NewRecorder()
	d.engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var doc struct {
		Keys []map[string]any `json:"keys"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &doc))
	kids := []string{}
	for _, k := range doc.Keys {
		for _, private := range []string{"d", "p", "q", "dp", "dq", "qi"} {
			require.NotContains(t, k, private, "el JWKS no puede publicar material privado")
		}
		kids = append(kids, k["kid"].(string))
	}
	return kids
}

// accepts resume los tres verificadores de iam para un token: Parse directo,
// gate de revocación (/me) y gate authorize (/users).
func (d deployment) accepts(t *testing.T, token string) (parseOK bool, me, users int) {
	t.Helper()
	_, err := d.verifier.Parse(token)
	return err == nil, d.get(t, "/api/v1/me", token), d.get(t, "/api/v1/users", token)
}

// fleetAccepts verifica como un consumidor de go-shared (TenantValidation /
// VerifyPlatformJWT usan jwtkeyring) sobre el mismo directorio montado.
func fleetAccepts(t *testing.T, dir, token string) bool {
	t.Helper()
	ring, err := jwtkeyring.Load(dir)
	require.NoError(t, err)
	// ParseDual es el selector que usan los middlewares de flota de go-shared
	// (el secreto se ignora desde T6: sólo RS256 por kid del keyring).
	err = jwtkeyring.ParseDual(token, jwt.MapClaims{}, "", ring)
	return err == nil
}

func TestRotacion_T8_SimulacroConSolapamiento(t *testing.T) {
	kid1 := config.SigningKeyKID(&key1.PublicKey)
	kid2 := config.SigningKeyKID(&key2.PublicKey)
	require.NotEqual(t, kid1, kid2)

	// Fase 0 — estado inicial: firma kid1, keyring {kid1}.
	d0 := deploy(t, key1, keyringDir(t, key1))
	tok1 := d0.login(t)
	assert.Equal(t, []string{kid1}, d0.jwksKIDs(t))

	// Fase 1 — PUBLICAR: kid2 entra al keyring; se sigue firmando con kid1.
	dirGracia := keyringDir(t, key1, key2)
	d1 := deploy(t, key1, dirGracia)
	assert.ElementsMatch(t, []string{kid1, kid2}, d1.jwksKIDs(t), "publicar = kid2 en el JWKS antes de firmar con ella")

	// Fase 2 — FIRMAR + GRACIA: el firmador pasa a kid2, el keyring mantiene kid1.
	d2 := deploy(t, key2, dirGracia)
	tok2 := d2.login(t)
	assert.ElementsMatch(t, []string{kid1, kid2}, d2.jwksKIDs(t), "C2: el JWKS publica exactamente la keyring aceptada")
	for name, tok := range map[string]string{"kid1": tok1, "kid2": tok2} {
		parseOK, me, users := d2.accepts(t, tok)
		assert.True(t, parseOK, "(a) Parse debe aceptar el token %s en gracia", name)
		assert.Equal(t, http.StatusOK, me, "(a) token_revocation debe aceptar %s en gracia", name)
		assert.Equal(t, http.StatusOK, users, "(a) authorize debe aceptar %s en gracia", name)
		assert.True(t, fleetAccepts(t, dirGracia, tok), "C4: la flota acepta %s en gracia", name)
	}

	// Fase 3 — RETIRAR: kid1 sale del keyring (de iam y de la flota).
	dirRetirado := keyringDir(t, key2)
	d3 := deploy(t, key2, dirRetirado)
	assert.Equal(t, []string{kid2}, d3.jwksKIDs(t), "kid1 retirado no puede seguir publicado")

	parseOK, me, users := d3.accepts(t, tok2)
	assert.True(t, parseOK)
	assert.Equal(t, http.StatusOK, me, "(b) kid2 sigue validando tras retirar kid1")
	assert.Equal(t, http.StatusOK, users, "(b) kid2 sigue validando tras retirar kid1")

	parseOK, me, users = d3.accepts(t, tok1)
	assert.False(t, parseOK, "(b) PRUEBA NEGATIVA: Parse no puede aceptar kid1 retirado — la rotación sería cosmética")
	assert.Equal(t, http.StatusUnauthorized, me, "(b) PRUEBA NEGATIVA: token_revocation rechaza kid1 retirado")
	assert.Equal(t, http.StatusUnauthorized, users, "(b) PRUEBA NEGATIVA: authorize rechaza kid1 retirado")
	assert.False(t, fleetAccepts(t, dirRetirado, tok1), "C4: la flota tampoco acepta kid1 retirado")
	assert.True(t, fleetAccepts(t, dirRetirado, tok2), "C4: la flota acepta kid2")
}

// C1: la carga de claves en gracia acepta SÓLO públicas — un PEM privado en
// el keyring es config rota y el arranque falla.
func TestRotacion_T8_KeyringConPrivadaFallaLaCarga(t *testing.T) {
	dir := keyringDir(t, key1)
	der, err := x509.MarshalPKCS8PrivateKey(key2)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "oops.pem"),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600))
	t.Setenv("JWT_PUBLIC_KEY_DIR", dir)
	_, err = config.LoadVerificationKeyringFromEnv()
	require.Error(t, err, "un PEM privado en el keyring debe rechazarse")
}

// Sin JWT_PUBLIC_KEY_DIR el verificador conserva el comportamiento previo:
// sólo la pública de firma.
func TestRotacion_T8_SinKeyringSoloLaDeFirma(t *testing.T) {
	t.Setenv("JWT_PUBLIC_KEY_DIR", "")
	ring, err := config.LoadVerificationKeyringFromEnv()
	require.NoError(t, err)
	assert.Nil(t, ring)
}
