// ACC-E03 T4 — el gate `authorize` verifica con la clave pública (RS256 por
// kid del header vía PublicKeyResolver, ADR-003 §f), manteniendo HS256 con el
// secreto viejo SOLO durante la ventana de cutover.
//
// Criterios "Hecho cuando" de T4 cubiertos acá:
//   - (a): un Bearer asimétrico válido (kid conocido) con rol permitido pasa el gate.
//   - (b): PRUEBA NEGATIVA — un Bearer `alg:none` → 401, y un Bearer asimétrico
//     firmado con OTRA clave → 401. Si cualquiera pasara, el gate de
//     autorización está ciego (bypass de auth).
package middleware_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authmw "github.com/hornosg/iam-service/src/access/infrastructure/middleware"
	"github.com/hornosg/iam-service/src/access/infrastructure/s2s"
	"github.com/hornosg/iam-service/src/identity/infrastructure/adapter"
	"github.com/hornosg/iam-service/src/identity/infrastructure/config"
	sharedctx "github.com/hornosg/iam-service/src/shared/context"
)

// t4Key y t4OtherKey son fixtures de test (no secretos reales): pares RSA
// generados acá, sólo circulan dentro de este archivo.
var (
	authorizeT4Key      = mustRSKey()
	authorizeT4OtherKey = mustRSKey()
)

func mustRSKey() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
}

// t4KID deriva el kid REAL del fixture (thumbprint RFC 7638) — el mismo
// mecanismo que usa el runtime; el test no inventa un esquema de kid.
func t4KID(t *testing.T, k *rsa.PrivateKey) string {
	t.Helper()
	kid := config.SigningKeyKID(&k.PublicKey)
	require.NotEmpty(t, kid)
	return kid
}

// t4Router arma el router con la factory REAL del gate (mismo harness que el
// archivo base) y la keyring RS256 del par dado. El endpoint reporta el flag
// system_admin derivado, para verificar además que la derivación de T8d sigue
// operando sobre claims verificados RS256.
func t4Router(t *testing.T, key *rsa.PrivateKey) (*gin.Engine, *map[string]bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()

	registry := s2s.LoadFromEnvForTests(map[string]string{
		"sales": salesKey,
	})
	verifier := adapter.NewJWTServiceAdapter(testJWTSecret, &adapter.SigningKey{
		PrivateKey: key,
		KID:        t4KID(t, key),
	})
	factory := authmw.NewScopeMiddlewareFactory(testJWTSecret, testNamespace, registry, verifier)

	report := map[string]bool{}
	reportFlag := func(c *gin.Context) {
		report["is_system_admin"] = sharedctx.IsSystemAdminFromContext(c.Request.Context())
		c.Status(http.StatusOK)
	}

	tenantScoped := router.Group("", factory.RequireScopes(
		[]s2s.Scope{s2s.ScopeSystemAdmin, s2s.ScopeTenantAdmin}, "tenant_admin", "system_admin"))
	tenantScoped.GET("/tenant-scoped", reportFlag)

	return router, &report
}

// t4SignRS firma un JWT RS256 (MapClaims, como los que Authorize consume) con
// el kid en el header.
func t4SignRS(t *testing.T, k *rsa.PrivateKey, roles []string, withKid bool) string {
	t.Helper()
	claims := jwt.MapClaims{
		"user_id":   uuid.New().String(),
		"tenant_id": uuid.New().String(),
		"roles":     roles,
		"namespace": testNamespace,
		"exp":       time.Now().Add(15 * time.Minute).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	if withKid {
		tok.Header["kid"] = t4KID(t, k)
	}
	str, err := tok.SignedString(k)
	require.NoError(t, err)
	return str
}

// CRITERIO (a) DE T4 — un Bearer asimétrico válido (kid conocido) con rol
// permitido pasa el gate, y la derivación del flag system_admin (T8d) sigue
// operando sobre el rol del token RS256.
func TestAuthorize_T4_RS256ValidoPasaGate(t *testing.T) {
	router, report := t4Router(t, authorizeT4Key)
	token := t4SignRS(t, authorizeT4Key, []string{"tenant_admin"}, true)
	w := do(t, router, http.MethodGet, "/tenant-scoped", token, "")
	assertAuthorized(t, w, "un Bearer RS256 válido (kid conocido) debe pasar el gate")
	assert.False(t, (*report)["is_system_admin"])

	tokenAdmin := t4SignRS(t, authorizeT4Key, []string{"system_admin"}, true)
	w = do(t, router, http.MethodGet, "/tenant-scoped", tokenAdmin, "")
	assertAuthorized(t, w, "un Bearer RS256 con system_admin debe pasar el gate tenant-scoped")
	assert.True(t, (*report)["is_system_admin"],
		"la derivación del flag T8d sigue operando sobre el rol del token RS256 verificado")
}

// CRITERIO (b) DE T4 — PRUEBA NEGATIVA: un Bearer `alg:none` → 401. Si
// pasara, el gate de autorización está ciego (bypass de auth).
func TestAuthorize_T4_AlgNoneRechazado401(t *testing.T) {
	router, _ := t4Router(t, authorizeT4Key)

	claims := jwt.MapClaims{
		"user_id":   uuid.New().String(),
		"tenant_id": uuid.New().String(),
		"roles":     []string{"tenant_admin"},
		"namespace": testNamespace,
		"exp":       time.Now().Add(15 * time.Minute).Unix(),
	}
	none := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	noneStr, err := none.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	w := do(t, router, http.MethodGet, "/tenant-scoped", noneStr, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code,
		"alg:none debe ser 401 (criterio (b) de T4): el gate jamás acepta tokens sin firma")
}

// CRITERIO (b) DE T4 — PRUEBA NEGATIVA: un Bearer asimétrico firmado con OTRA
// clave → 401. Si pasara, el gate no está verificando la firma.
func TestAuthorize_T4_OtraClaveRechazada401(t *testing.T) {
	router, _ := t4Router(t, authorizeT4Key)

	// Válido en forma (kid del token = kid de la keyring) pero firmado con
	// la clave que la keyring NO conoce: la firma no verifica.
	token := t4SignRS(t, authorizeT4OtherKey, []string{"tenant_admin"}, true)
	w := do(t, router, http.MethodGet, "/tenant-scoped", token, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code,
		"un token RS256 firmado con OTRA clave debe ser 401 (criterio (b) de T4)")
}

// Kid de la keyring con firma de otra clave es el caso más trampa: el kid
// conocido hace el lookup, la firma RSA no verifica. 401 igual.
func TestAuthorize_T4_KidConocidoFirmaAjenaRechazada401(t *testing.T) {
	router, _ := t4Router(t, authorizeT4Key)

	claims := jwt.MapClaims{
		"user_id":   uuid.New().String(),
		"tenant_id": uuid.New().String(),
		"roles":     []string{"tenant_admin"},
		"namespace": testNamespace,
		"exp":       time.Now().Add(15 * time.Minute).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = t4KID(t, authorizeT4Key)      // kid de la keyring...
	str, err := tok.SignedString(authorizeT4OtherKey) // ...firmado con la otra
	require.NoError(t, err)

	w := do(t, router, http.MethodGet, "/tenant-scoped", str, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code,
		"kid conocido no vale nada si la firma no verifica contra la pública de ese kid")
}

// RS256 sin kid en el header → 401 (la selección §f es por kid; sin kid no
// hay clave que elegir y el gate falla cerrado).
func TestAuthorize_T4_RS256SinKidRechazado401(t *testing.T) {
	router, _ := t4Router(t, authorizeT4Key)
	token := t4SignRS(t, authorizeT4Key, []string{"tenant_admin"}, false)
	w := do(t, router, http.MethodGet, "/tenant-scoped", token, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code, "RS256 sin kid en el header → 401")
}

// CRITERIO "jamás confusión RS/HS" DE T4 — PRUEBA NEGATIVA a nivel de GATE: un
// token HS256 forjado usando el PEM de la pública de la keyring como secreto
// HMAC → 401. La rama HS256 del keyFunc dual devuelve SÓLO el secreto legacy
// (comentario del gate de T4); si verificara contra el PEM público, un atacante
// con la clave pública —material público por definición— podría firmar tokens
// válidos. El test de T3 cubre la misma trampa a nivel `Parse` del firmador;
// acá se cuelga del keyFunc del propio gate, que es el control que T4 cambia.
func TestAuthorize_T4_ConfusionRSHSRechazada401(t *testing.T) {
	router, _ := t4Router(t, authorizeT4Key)

	pubDER, err := x509.MarshalPKIXPublicKey(&authorizeT4Key.PublicKey)
	require.NoError(t, err)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	require.NotNil(t, pubPEM)

	claims := jwt.MapClaims{
		"user_id":   uuid.New().String(),
		"tenant_id": uuid.New().String(),
		"roles":     []string{"system_admin"},
		"namespace": testNamespace,
		"exp":       time.Now().Add(15 * time.Minute).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	str, err := tok.SignedString(pubPEM)
	require.NoError(t, err, "el forjado HS256-con-PEM-público debe producir un token bien formado: la trampa está en la VERIFICACIÓN, no en la construcción")

	w := do(t, router, http.MethodGet, "/tenant-scoped", str, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code,
		"HS256 firmado con la pública como secreto debe ser 401 (confusión RS/HS cerrada a nivel gate)")
}

// VENTANA DUAL — el HS256 del secreto viejo sigue autorizando hasta T6
// (ADR-003 §f): la paridad con el harness base queda pinneada junto a los
// RS256 para que el archivo cuente la historia completa del gate dual.
func TestAuthorize_T4_HS256LegacySiguePasando(t *testing.T) {
	router, report := t4Router(t, authorizeT4Key)
	token := makeToken(t, []string{"tenant_admin"}, nil)
	w := do(t, router, http.MethodGet, "/tenant-scoped", token, "")
	assertAuthorized(t, w, "durante la ventana de cutover el HS256 legacy sigue autorizado")
	assert.False(t, (*report)["is_system_admin"])
}
