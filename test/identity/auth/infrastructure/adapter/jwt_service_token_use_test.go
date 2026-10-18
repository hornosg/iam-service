package adapter_test

// ACC-E09 T2 — separación de tipos de token (ADR-006).
//
// El token de servicio se firma con la MISMA clave privada de iam que los
// tokens de usuario. Sin el corte de userClaims, un token de servicio con un
// `kid` válido pasaría Parse/ParseIgnoringExpiry (y por ellos token_revocation)
// como si fuera de un usuario. Estos negativos firman con la clave legítima a
// propósito: lo único que los distingue es `token_use`/`iss`.

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/hornosg/go-shared/infrastructure/s2stoken"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hornosg/iam-service/src/identity/infrastructure/adapter"
	"github.com/hornosg/iam-service/src/identity/infrastructure/config"
)

// signRaw firma claims arbitrarios con RS256 y el kid dado.
func signRaw(t *testing.T, key *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(key)
	require.NoError(t, err)
	return s
}

func userLikeClaims(exp time.Time) jwt.MapClaims {
	return jwt.MapClaims{
		"jti":       uuid.NewString(),
		"iss":       s2stoken.UserIssuer,
		"namespace": "mc",
		"user_id":   uuid.NewString(),
		"email":     "user@example.com",
		"tenant_id": uuid.NewString(),
		"roles":     []string{"system_admin"},
		"iat":       time.Now().Unix(),
		"exp":       exp.Unix(),
	}
}

func TestJWTServiceParseRejectsServiceTokens(t *testing.T) {
	sk, _ := newSigningFixture(t)
	svc := adapter.NewJWTServiceAdapter(sk)

	cases := []struct {
		name   string
		mutate func(c jwt.MapClaims)
	}{
		{"token de servicio completo", func(c jwt.MapClaims) {
			c["iss"] = s2stoken.Issuer
			c["token_use"] = s2stoken.TokenUse
			c["aud"] = s2stoken.IAMAudience
			c["sub"] = "catalog-bff-service"
			c["tenant_scope"] = s2stoken.TenantScopeSystem
		}},
		{"token_use=service con iss de usuario", func(c jwt.MapClaims) {
			c["token_use"] = s2stoken.TokenUse
		}},
		{"iss de servicio sin token_use", func(c jwt.MapClaims) {
			c["iss"] = s2stoken.Issuer
		}},
		{"sin iss", func(c jwt.MapClaims) {
			delete(c, "iss")
		}},
		{"iss ajeno", func(c jwt.MapClaims) {
			c["iss"] = "otro-emisor"
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Vigente: Parse debe rechazarlo por tipo, no por expiración.
			live := userLikeClaims(time.Now().Add(5 * time.Minute))
			tc.mutate(live)
			tok := signRaw(t, sk.PrivateKey, sk.KID, live)

			got, err := svc.Parse(tok)
			assert.Error(t, err, "Parse debe rechazar")
			assert.Nil(t, got)

			got, err = svc.ParseIgnoringExpiry(tok)
			assert.Error(t, err, "ParseIgnoringExpiry debe rechazar")
			assert.Nil(t, got)

			// Vencido: ParseIgnoringExpiry no valida exp, así que sólo el corte de
			// tipo lo frena (el camino del presenter de /auth/refresh).
			expired := userLikeClaims(time.Now().Add(-time.Hour))
			tc.mutate(expired)
			got, err = svc.ParseIgnoringExpiry(signRaw(t, sk.PrivateKey, sk.KID, expired))
			assert.Error(t, err, "ParseIgnoringExpiry debe rechazar el token vencido por tipo")
			assert.Nil(t, got)
		})
	}
}

// El positivo de control: el mismo armado con iss de usuario y sin token_use
// pasa. Sin él, los negativos de arriba pasarían también con un Parse roto.
func TestJWTServiceParseAcceptsUserTokenControl(t *testing.T) {
	sk, _ := newSigningFixture(t)
	svc := adapter.NewJWTServiceAdapter(sk)
	tok := signRaw(t, sk.PrivateKey, sk.KID, userLikeClaims(time.Now().Add(5*time.Minute)))

	got, err := svc.Parse(tok)
	require.NoError(t, err)
	assert.Equal(t, s2stoken.UserIssuer, got.Issuer)

	_, err = svc.ParseIgnoringExpiry(tok)
	require.NoError(t, err)
}

// Un token firmado con la clave privada de un EMISOR (su par de s2s-keygen) da
// 401 en Parse aunque declare un kid derivado de su propio material y claims de
// usuario: las públicas de emisores nunca entran en el keyring de verificación.
func TestJWTServiceParseRejectsTokenSignedWithIssuerKey(t *testing.T) {
	sk, _ := newSigningFixture(t)
	svc := adapter.NewJWTServiceAdapter(sk)

	issuerKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	issuerKID := config.SigningKeyKID(&issuerKey.PublicKey)

	tok := signRaw(t, issuerKey, issuerKID, userLikeClaims(time.Now().Add(5*time.Minute)))
	got, err := svc.Parse(tok)
	assert.Error(t, err)
	assert.Nil(t, got)

	// Ni siquiera reutilizando el kid vigente de iam.
	tok = signRaw(t, issuerKey, sk.KID, userLikeClaims(time.Now().Add(5*time.Minute)))
	got, err = svc.Parse(tok)
	assert.Error(t, err)
	assert.Nil(t, got)
}
