package servicetoken

import (
	"crypto/rsa"
	"errors"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/hornosg/go-shared/infrastructure/s2stoken"
	"github.com/hornosg/iam-service/src/access/domain/entity"
	"github.com/hornosg/iam-service/src/access/domain/port"
)

// RS256Signer firma tokens de servicio con la clave privada vigente de iam (la misma de los
// tokens de usuario, ADR-003). El composition root le pasa la clave cruda: access no importa
// identity. Sin rama HMAC.
type RS256Signer struct {
	key *rsa.PrivateKey
	kid string
}

var _ port.ServiceTokenSigner = (*RS256Signer)(nil)

func NewRS256Signer(key *rsa.PrivateKey, kid string) *RS256Signer {
	return &RS256Signer{key: key, kid: kid}
}

func (s *RS256Signer) SignServiceToken(c entity.ServiceTokenClaims) (string, string, error) {
	if s == nil || s.key == nil || s.kid == "" {
		return "", "", errors.New("s2s signer: sin clave de firma")
	}
	claims := &s2stoken.Claims{
		Issuer:      c.Issuer,
		Subject:     c.Subject,
		Audience:    c.Audience,
		Scope:       strings.Join(c.Scopes, " "),
		TokenUse:    c.TokenUse,
		TenantScope: c.TenantScope,
		Namespace:   c.Namespace,
		IssuedAt:    jwt.NewNumericDate(c.IssuedAt),
		NotBefore:   jwt.NewNumericDate(c.IssuedAt),
		ExpiresAt:   jwt.NewNumericDate(c.ExpiresAt),
		ID:          c.JTI,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = s.kid
	tok.Header["typ"] = s2stoken.TokenType
	signed, err := tok.SignedString(s.key)
	if err != nil {
		return "", "", err
	}
	return signed, s.kid, nil
}
