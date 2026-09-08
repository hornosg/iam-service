package adapter_test

// ACC-E03 T3 — firmador asimétrico con kid + Parse con aceptación dual.
//
// Este archivo verifica los criterios del "Hecho cuando" de T3 contra el
// adapter REAL (no mocks), incluidas las tres pruebas negativas que la épica
// exige porque un test de round-trip feliz pasa igual con la trampa presente:
//
//	(b1) alg:none                 → Parse error, sin claims
//	(b2) HS256 "firmado" con el PEM público como secreto (confusión RS/HS)
//	(c)  RS256 firmado con OTRA clave privada
//
// La aceptación dual (HS256 con el secreto viejo) es la ventana de cutover
// de ADR-003 §f: se prueba en positivo acá y T6 la retira.

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hornosg/iam-service/src/identity/domain/value_object"
	"github.com/hornosg/iam-service/src/identity/infrastructure/adapter"
	"github.com/hornosg/iam-service/src/identity/infrastructure/config"
)

const testLegacySecret = "test-legacy-hs256-secret-32-chars-long"

// newSigningFixture genera el par asimétrico del "firmador legítimo" y su kid
// derivado con la MISMA función del runtime.
func newSigningFixture(t *testing.T) (*adapter.SigningKey, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return &adapter.SigningKey{
		PrivateKey: key,
		KID:        config.SigningKeyKID(&key.PublicKey),
	}, other
}

func validTestClaims() value_object.TokenClaims {
	return value_object.TokenClaims{
		JTI:       uuid.New(),
		Issuer:    "iam-service",
		Namespace: "mc",
		UserID:    uuid.New(),
		Email:     "user@example.com",
		TenantID:  uuid.New(),
		RoleID:    uuid.New(),
		ExpiresAt: time.Now().Add(15 * time.Minute).Unix(),
		IssuedAt:  time.Now().Unix(),
	}
}

// criteria (a): el header decodificado del token de Sign tiene alg asimétrico
// y kid no vacío.
func TestJWTServiceSignProducesRS256WithKID(t *testing.T) {
	key, _ := newSigningFixture(t)
	svc := adapter.NewJWTServiceAdapter(testLegacySecret, key)

	token, err := svc.Sign(&value_object.TokenClaims{})
	require.NoError(t, err)

	parts := strings.Split(token, ".")
	require.Len(t, parts, 3, "un JWT RS256 tiene tres partes")

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	require.NoError(t, err)
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	require.NoError(t, json.Unmarshal(headerJSON, &header))

	assert.Equal(t, "RS256", header.Alg, "el firmador debe usar el algoritmo asimétrico del ADR-003 §a")
	assert.Equal(t, key.KID, header.Kid, "el kid del header debe ser el registrado para el par")
	assert.NotEmpty(t, header.Kid)
}

// criteria (d): round-trip — el token de Sign, pasado a Parse, devuelve los
// mismos claims.
func TestJWTServiceSignRoundTripPreservesClaims(t *testing.T) {
	key, _ := newSigningFixture(t)
	svc := adapter.NewJWTServiceAdapter(testLegacySecret, key)
	claims := validTestClaims()

	token, err := svc.Sign(&claims)
	require.NoError(t, err)

	parsed, err := svc.Parse(token)
	require.NoError(t, err)
	assert.Equal(t, claims.JTI, parsed.JTI)
	assert.Equal(t, claims.UserID, parsed.UserID)
	assert.Equal(t, claims.TenantID, parsed.TenantID)
	assert.Equal(t, claims.RoleID, parsed.RoleID)
	assert.Equal(t, claims.Email, parsed.Email)
	assert.Equal(t, claims.ExpiresAt, parsed.ExpiresAt)
}

// Caveat 2 de T2: Sign con signingKey nil falla explícito — nunca firma
// "algo" con un firmador no configurado.
func TestJWTServiceSignFailsWithoutSigningKey(t *testing.T) {
	svc := adapter.NewJWTServiceAdapter(testLegacySecret, nil)

	token, err := svc.Sign(&value_object.TokenClaims{})
	assert.Error(t, err, "firmar sin clave configurada debe ser un error explícito")
	assert.Empty(t, token)
}

// Aceptación dual (ADR-003 §f, rama HS256): durante la ventana de cutover un
// token HS256 firmado con el secreto viejo sigue validando in-process.
func TestJWTServiceParseAcceptsLegacyHS256(t *testing.T) {
	key, _ := newSigningFixture(t)
	svc := adapter.NewJWTServiceAdapter(testLegacySecret, key)
	claims := validTestClaims()

	legacyToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, adapter.JWTClaims{TokenClaims: claims}).SignedString([]byte(testLegacySecret))
	require.NoError(t, err)

	parsed, err := svc.Parse(legacyToken)
	require.NoError(t, err, "la aceptación dual §f mantiene vivos los tokens HS256 del secreto viejo hasta T6")
	assert.Equal(t, claims.UserID, parsed.UserID)
}

// Criteria (b1): token alg:none → error, sin claims. Un verificador que no
// fija el método aceptaría tokens sin firma.
func TestJWTServiceParseRejectsAlgNone(t *testing.T) {
	key, _ := newSigningFixture(t)
	claims := validTestClaims()

	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, adapter.JWTClaims{TokenClaims: claims}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	for name, parse := range map[string]func(string) (*value_object.TokenClaims, error){
		"Parse":              func(s string) (*value_object.TokenClaims, error) { return adapter.NewJWTServiceAdapter(testLegacySecret, key).Parse(s) },
		"ParseIgnoringExpiry": func(s string) (*value_object.TokenClaims, error) { return adapter.NewJWTServiceAdapter(testLegacySecret, key).ParseIgnoringExpiry(s) },
	} {
		parsed, err := parse(unsigned)
		assert.Error(t, err, "%s debe rechazar alg:none", name)
		assert.Nil(t, parsed, "%s no debe devolver claims para alg:none", name)
	}
}

// Criteria (b2): PRUEBA NEGATIVA de confusión RS/HS — un token HS256 forjado
// usando el PEM público como secreto HMAC. La rama HS256 del keyfunc devuelve
// SÓLO el secreto viejo, así que la firma forjada no puede verificar.
func TestJWTServiceParseRejectsHS256SignedWithPublicKey(t *testing.T) {
	key, _ := newSigningFixture(t)
	claims := validTestClaims()

	pubDER, err := x509.MarshalPKIXPublicKey(&key.PrivateKey.PublicKey)
	require.NoError(t, err)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	confused, err := jwt.NewWithClaims(jwt.SigningMethodHS256, adapter.JWTClaims{TokenClaims: claims}).SignedString(pubPEM)
	require.NoError(t, err)

	svc := adapter.NewJWTServiceAdapter(testLegacySecret, key)
	parsed, err := svc.Parse(confused)
	assert.Error(t, err, "un token HS256 firmado con la clave pública como secreto es la confusión RS/HS — debe fallar")
	assert.Nil(t, parsed, "la confusión RS/HS no debe entregar claims")
}

// Criteria (c): PRUEBA NEGATIVA de firma — un token RS256 con el kid legítimo
// en el header pero firmado con OTRA clave privada. El keyfunc resuelve por
// kid la pública legítima y la verificación de firma falla.
func TestJWTServiceParseRejectsTokenSignedWithOtherKey(t *testing.T) {
	key, other := newSigningFixture(t)
	claims := validTestClaims()

	forged := jwt.NewWithClaims(jwt.SigningMethodRS256, adapter.JWTClaims{TokenClaims: claims})
	forged.Header["kid"] = key.KID
	forgedToken, err := forged.SignedString(other)
	require.NoError(t, err)

	svc := adapter.NewJWTServiceAdapter(testLegacySecret, key)
	parsed, err := svc.Parse(forgedToken)
	assert.Error(t, err, "un token firmado con otra clave privada debe fallar verificación")
	assert.Nil(t, parsed, "un token de otra clave no debe entregar claims")
}

// Un token RS256 sin kid no puede resolverse por la regla §f (selección por
// header) — se rechaza aunque la firma sea de la clave legítima.
func TestJWTServiceParseRejectsRS256WithoutKID(t *testing.T) {
	key, _ := newSigningFixture(t)
	claims := validTestClaims()

	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, adapter.JWTClaims{TokenClaims: claims}).SignedString(key.PrivateKey)
	require.NoError(t, err)

	svc := adapter.NewJWTServiceAdapter(testLegacySecret, key)
	parsed, err := svc.Parse(token)
	assert.Error(t, err, "RS256 sin kid no es seleccionable por la regla dual §f")
	assert.Nil(t, parsed)
}