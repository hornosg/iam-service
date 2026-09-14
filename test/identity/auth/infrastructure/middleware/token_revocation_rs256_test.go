// ACC-E03 T4 — el gate de revocación verifica con la clave pública (RS256 por
// kid del header, ADR-003 §f), manteniendo HS256 con el secreto viejo SOLO
// durante la ventana de cutover.
//
// Criterios "Hecho cuando" de T4 cubiertos acá:
//   - (a): un Bearer asimétrico válido (kid conocido) puebla user_id/token_claims.
//   - (c): PRUEBA NEGATIVA — un token con firma inválida (otra clave) NO puebla
//     token_claims: el request no queda autenticado (el middleware continúa,
//     como hoy con un token HS256 inválido — la decisión de 401 la toman los
//     gates de autorización downstream, contrato del gate de revocación).
package middleware_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hornosg/iam-service/src/identity/domain/port"
	"github.com/hornosg/iam-service/src/identity/domain/value_object"
	"github.com/hornosg/iam-service/src/identity/infrastructure/adapter"
	"github.com/hornosg/iam-service/src/identity/infrastructure/config"
	"github.com/hornosg/iam-service/src/identity/infrastructure/middleware"
	sharedctx "github.com/hornosg/iam-service/src/shared/context"
	repo "github.com/hornosg/iam-service/test/identity/auth/infrastructure/persistence/repository"
)

// t4Key y t4OtherKey son fixtures de test (no secretos reales): pares RSA
// generados acá, sólo circulan dentro de este archivo.
var (
	t4Key      = mustRSAKey()
	t4OtherKey = mustRSAKey()
)

func mustRSAKey() *rsa.PrivateKey {
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

// t4Sign firma un JWT RS256 con el kid en el header y los claims dados.
func t4Sign(t *testing.T, k *rsa.PrivateKey, claims value_object.TokenClaims) string {
	t.Helper()
	c := &adapter.JWTClaims{TokenClaims: claims}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
	tok.Header["kid"] = t4KID(t, k)
	str, err := tok.SignedString(k)
	require.NoError(t, err)
	return str
}

// t4NoneToken firma un JWT con alg "none".
func t4NoneToken(t *testing.T, claims value_object.TokenClaims) string {
	t.Helper()
	c := &adapter.JWTClaims{TokenClaims: claims}
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, c)
	str, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)
	return str
}

// t4Verifier arma el verificador dual (T3/T4) con el par RSA del fixture y el
// secreto legacy del archivo base.
func t4Verifier(t *testing.T) *adapter.JWTServiceAdapter {
	return adapter.NewJWTServiceAdapter(signingKey, &adapter.SigningKey{
		PrivateKey: t4Key,
		KID:        t4KID(t, t4Key),
	})
}

// t4Engine arma el middleware bajo test con el verificador dual dado. El
// handler final refleja en headers qué quedó poblado en el contexto, para que
// cada test assertee sin duplicar el harness del archivo base.
func t4Engine(t *testing.T, verifier port.JWTService) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.TokenRevocationCheck(middleware.TokenRevocationConfig{
		JWT:      verifier,
		AuthRepo: repo.NewMockAuthRepository(),
	}))
	r.GET("/*p", func(c *gin.Context) {
		if uid, ok := c.Get("user_id"); ok {
			c.Header("X-User-Id", uid.(uuid.UUID).String())
		}
		if tid, ok := sharedctx.TenantIDFromContext(c.Request.Context()); ok {
			c.Header("X-Tenant-Id", tid.String())
		}
		if tc, ok := c.Get("token_claims"); ok {
			c.Header("X-Token-Claims", tc.(*value_object.TokenClaims).JTI.String())
		}
		c.Status(http.StatusOK)
	})
	return r
}

// CRITERIO (a) DE T4 — un Bearer asimétrico válido (kid conocido) puebla
// user_id, tenant (contexto RLS) y token_claims.
func TestTokenRevocation_T4_RS256ValidoPueblaClaims(t *testing.T) {
	r := t4Engine(t, t4Verifier(t))

	claims := baseClaims(uuid.New())
	w := doRequest(t, r, "/api/v1/anything", "Bearer "+t4Sign(t, t4Key, claims))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, claims.UserID.String(), w.Header().Get("X-User-Id"),
		"un token RS256 válido (kid conocido) debe poblar user_id")
	assert.Equal(t, claims.TenantID.String(), w.Header().Get("X-Tenant-Id"),
		"el tenant del claim verificado debe propagarse al context.Context (T8e)")
	assert.Equal(t, claims.JTI.String(), w.Header().Get("X-Token-Claims"),
		"un token RS256 válido debe poblar token_claims")
}

// CRITERIO (c) DE T4 — PRUEBA NEGATIVA: un token asimétrico firmado con OTRA
// clave NO puebla token_claims (ni user_id ni tenant): el request no queda
// autenticado. El middleware continúa (200 del handler) porque la decisión de
// 401 pertenece a los gates de autorización — pero sin claims.
func TestTokenRevocation_T4_FirmaInvalidaNoPueblaTokenClaims(t *testing.T) {
	r := t4Engine(t, t4Verifier(t))

	// Firmado con OTRA clave: la firma no verifica contra la keyring.
	w := doRequest(t, r, "/api/v1/anything", "Bearer "+t4Sign(t, t4OtherKey, baseClaims(uuid.New())))
	require.Equal(t, http.StatusOK, w.Code, "el gate de revocación no decide 401; continúa sin autenticar")
	assert.Empty(t, w.Header().Get("X-Token-Claims"),
		"firma inválida NO debe poblar token_claims (criterio (c) de T4)")
	assert.Empty(t, w.Header().Get("X-User-Id"), "firma inválida NO debe poblar user_id")
	assert.Empty(t, w.Header().Get("X-Tenant-Id"), "firma inválida NO debe propagar tenant")

	// Y `alg:none`: tampoco.
	w = doRequest(t, r, "/api/v1/anything", "Bearer "+t4NoneToken(t, baseClaims(uuid.New())))
	assert.Empty(t, w.Header().Get("X-Token-Claims"),
		"alg:none NO debe poblar token_claims")
}

// CRITERIO "jamás confusión RS/HS" DE T4 — PRUEBA NEGATIVA a nivel de GATE: un
// token HS256 forjado con el PEM de la pública de la keyring como secreto HMAC
// NO puebla token_claims (ni user_id ni tenant): el request no queda
// autenticado. El verificador dual es la MISMA instancia del firmador (comentario
// del cfg de T4), cuya rama HS256 sólo acepta el secreto legacy — el test de T3
// cubre la trampa a nivel `Parse`; acá se pega al flujo completo del gate de
// revocación, que es el control que T4 cambia.
func TestTokenRevocation_T4_ConfusionRSHSNoPueblaTokenClaims(t *testing.T) {
	r := t4Engine(t, t4Verifier(t))

	pubDER, err := x509.MarshalPKIXPublicKey(&t4Key.PublicKey)
	require.NoError(t, err)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	require.NotNil(t, pubPEM)

	c := &adapter.JWTClaims{TokenClaims: baseClaims(uuid.New())}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	str, err := tok.SignedString(pubPEM)
	require.NoError(t, err, "el forjado HS256-con-PEM-público debe producir un token bien formado: la trampa está en la VERIFICACIÓN, no en la construcción")

	w := doRequest(t, r, "/api/v1/anything", "Bearer "+str)
	assert.Empty(t, w.Header().Get("X-Token-Claims"),
		"HS256 firmado con la pública como secreto NO debe poblar token_claims (confusión RS/HS cerrada a nivel gate)")
	assert.Empty(t, w.Header().Get("X-User-Id"), "el request no queda autenticado")
}

// VENTANA DUAL — el token HS256 del secreto viejo sigue poblado hasta T6
// (paridad con el harness base, que ya lo cubre; acá queda pinneado junto a
// los RS256 para que el archivo cuente la historia completa del gate dual).
func TestTokenRevocation_T4_HS256LegacySiguePoblando(t *testing.T) {
	r := t4Engine(t, t4Verifier(t))

	claims := baseClaims(uuid.New())
	w := doRequest(t, r, "/api/v1/anything", "Bearer "+signToken(t, claims))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, claims.UserID.String(), w.Header().Get("X-User-Id"),
		"durante la ventana de cutover el HS256 legacy sigue verificado (ADR-003 §f)")
}

// interface-check: la superficie del gate sigue siendo port.JWTService.
var _ port.JWTService = (*adapter.JWTServiceAdapter)(nil)