package servicetoken

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/hornosg/go-shared/infrastructure/s2stoken"
	"github.com/hornosg/iam-service/src/access/domain/entity"
	"github.com/hornosg/iam-service/src/access/domain/exception"
	"github.com/hornosg/iam-service/src/access/domain/port"
)

// maxAssertionLifetime: exp − iat ≤ 60 s (ADR-006 «Emisión y renovación»).
const maxAssertionLifetime = 60 * time.Second

// AssertionVerifier verifica la client_assertion (private_key_jwt) contra el registro de
// públicas de emisores, nunca contra el keyring de verificación de iam.
type AssertionVerifier struct {
	keys port.IssuerKeyRegistry
	now  func() time.Time
}

var _ port.ClientAssertionVerifier = (*AssertionVerifier)(nil)

func NewAssertionVerifier(keys port.IssuerKeyRegistry, now func() time.Time) *AssertionVerifier {
	if now == nil {
		now = time.Now
	}
	return &AssertionVerifier{keys: keys, now: now}
}

func reject(format string, args ...any) error {
	return fmt.Errorf("%w: %s", exception.ErrServiceTokenUnauthenticated, fmt.Sprintf(format, args...))
}

// Verify aplica el orden fijo: forma → firma por (iss=sub, kid) → exp/iat.
func (v *AssertionVerifier) Verify(raw string) (entity.VerifiedAssertion, error) {
	// 1. Forma, sin confiar en nada todavía: sólo decide qué clave buscar.
	var shape s2stoken.Claims
	unverified, _, err := jwt.NewParser().ParseUnverified(raw, &shape)
	if err != nil {
		return entity.VerifiedAssertion{}, reject("aserción malformada")
	}
	if alg, _ := unverified.Header["alg"].(string); alg != jwt.SigningMethodRS256.Alg() {
		return entity.VerifiedAssertion{}, reject("alg %q no admitido", alg)
	}
	kid, _ := unverified.Header["kid"].(string)
	if kid == "" {
		return entity.VerifiedAssertion{}, reject("aserción sin kid")
	}
	if shape.Issuer == "" || shape.Issuer != shape.Subject {
		return entity.VerifiedAssertion{}, reject("iss y sub deben ser el emisor")
	}
	if shape.Audience != s2stoken.AssertionAudience {
		return entity.VerifiedAssertion{}, reject("aud de aserción inválido")
	}
	if strings.TrimSpace(shape.ID) == "" || shape.IssuedAt == nil || shape.ExpiresAt == nil {
		return entity.VerifiedAssertion{}, reject("jti, iat y exp son obligatorios")
	}
	if shape.ExpiresAt.Sub(shape.IssuedAt.Time) > maxAssertionLifetime || !shape.ExpiresAt.After(shape.IssuedAt.Time) {
		return entity.VerifiedAssertion{}, reject("vida de aserción fuera de (0, 60s]")
	}

	// 2. Firma: el kid se busca sólo dentro del emisor que declara iss=sub.
	pub, ok := v.keys.PublicKey(shape.Issuer, kid)
	if !ok {
		return entity.VerifiedAssertion{}, reject("emisor o kid no registrado")
	}
	// 3. exp/iat con leeway, ya sobre los claims firmados.
	var claims s2stoken.Claims
	_, err = jwt.ParseWithClaims(raw, &claims,
		func(*jwt.Token) (interface{}, error) { return pub, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithLeeway(s2stoken.Leeway),
		jwt.WithIssuedAt(),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(v.now),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return entity.VerifiedAssertion{}, reject("aserción vencida")
		}
		return entity.VerifiedAssertion{}, reject("firma o tiempos inválidos")
	}
	return entity.VerifiedAssertion{Issuer: claims.Issuer, KID: kid, JTI: claims.ID, ExpiresAt: claims.ExpiresAt.Time}, nil
}
