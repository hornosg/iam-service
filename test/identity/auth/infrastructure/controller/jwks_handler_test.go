package controller_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hornosg/iam-service/src/identity/domain/value_object"
	"github.com/hornosg/iam-service/src/identity/infrastructure/adapter"
	"github.com/hornosg/iam-service/src/identity/infrastructure/config"
	"github.com/hornosg/iam-service/src/identity/infrastructure/controller"
)

// ACC-E03 T7: JWKS público (ADR-003 §d).

func jwksEngine(p controller.JWKSProvider) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET(controller.JWKSPath, controller.JWKSHandler(p))
	return r
}

func getJWKS(t *testing.T, p controller.JWKSProvider) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	jwksEngine(p).ServeHTTP(w, httptest.NewRequest(http.MethodGet, controller.JWKSPath, nil))
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return w, body
}

func TestJWKS_SirveLaClaveActivaConSuKid(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	kid := config.SigningKeyKID(&key.PublicKey)
	svc := adapter.NewJWTServiceAdapter(&adapter.SigningKey{PrivateKey: key, KID: kid})

	w, body := getJWKS(t, svc)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "public, max-age=300", w.Header().Get("Cache-Control"))
	keys := body["keys"].([]any)
	require.Len(t, keys, 1)
	jwk := keys[0].(map[string]any)
	assert.Equal(t, "RSA", jwk["kty"])
	assert.Equal(t, "sig", jwk["use"])
	assert.Equal(t, "RS256", jwk["alg"])
	assert.Equal(t, kid, jwk["kid"])

	// n/e reconstruyen exactamente la pública de la clave de firma.
	nb, err := base64.RawURLEncoding.DecodeString(jwk["n"].(string))
	require.NoError(t, err)
	eb, err := base64.RawURLEncoding.DecodeString(jwk["e"].(string))
	require.NoError(t, err)
	pub := &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: int(new(big.Int).SetBytes(eb).Int64())}
	assert.True(t, pub.Equal(&key.PublicKey))
	// y el kid publicado es el thumbprint de ESE material (ADR-003 §c).
	assert.Equal(t, kid, config.SigningKeyKID(pub))
}

// El kid del JWKS coincide con el del header de un token recién firmado, y la
// pública publicada lo verifica — un consumidor fuera de Kong puede validar
// con sólo el JWKS.
func TestJWKS_KidCoincideConElHeaderYVerificaElToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	svc := adapter.NewJWTServiceAdapter(&adapter.SigningKey{PrivateKey: key, KID: config.SigningKeyKID(&key.PublicKey)})
	tok, err := svc.Sign(&value_object.TokenClaims{UserID: uuid.New()})
	require.NoError(t, err)

	doc := svc.PublicJWKS()
	require.Len(t, doc.Keys, 1)
	parsed, err := jwt.Parse(tok, func(jt *jwt.Token) (interface{}, error) {
		assert.Equal(t, doc.Keys[0].Kid, jt.Header["kid"])
		nb, _ := base64.RawURLEncoding.DecodeString(doc.Keys[0].N)
		eb, _ := base64.RawURLEncoding.DecodeString(doc.Keys[0].E)
		return &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: int(new(big.Int).SetBytes(eb).Int64())}, nil
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithoutClaimsValidation())
	require.NoError(t, err)
	assert.True(t, parsed.Valid)
}

// PRUEBA NEGATIVA de fuga (criterio c): el cuerpo no lleva `d`/`p`/`q`/`dp`/
// `dq`/`qi` ni PEM privado. Se chequea el cuerpo crudo, no el struct: un
// campo agregado por error al JWK aparecería acá.
func TestJWKS_NoExponeMaterialPrivado(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	svc := adapter.NewJWTServiceAdapter(&adapter.SigningKey{PrivateKey: key, KID: config.SigningKeyKID(&key.PublicKey)})

	w := httptest.NewRecorder()
	jwksEngine(svc).ServeHTTP(w, httptest.NewRequest(http.MethodGet, controller.JWKSPath, nil))
	raw := w.Body.String()

	leak := regexp.MustCompile(`(?i)"(d|p|q|dp|dq|qi)"\s*:|PRIVATE`)
	assert.False(t, leak.MatchString(raw), "el JWKS expone material privado: %s", raw)
	// el exponente privado tampoco aparece en ninguna codificación base64url.
	assert.NotContains(t, raw, base64.RawURLEncoding.EncodeToString(key.D.Bytes()))
}

// Sin clave de firma (dev sin JWT_PRIVATE_KEY_FILE) y con receptor nil
// (identity desmontado, T4b): JWKS vacío, nunca panic.
func TestJWKS_SinClaveDevuelveKeysVacio(t *testing.T) {
	w, body := getJWKS(t, adapter.NewJWTServiceAdapter(nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []any{}, body["keys"])

	var nilSvc *adapter.JWTServiceAdapter
	assert.Empty(t, nilSvc.PublicJWKS().Keys)
}
