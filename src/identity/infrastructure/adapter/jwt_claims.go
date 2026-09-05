package adapter

import (
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/hornosg/iam-service/src/identity/domain/value_object"
)

// JWTClaims wraps TokenClaims to implement jwt.Claims, keeping jwt library out of domain.
type JWTClaims struct {
	value_object.TokenClaims
}

func (c JWTClaims) GetExpirationTime() (*jwt.NumericDate, error) {
	if c.ExpiresAt == 0 {
		return nil, nil
	}
	return jwt.NewNumericDate(time.Unix(c.ExpiresAt, 0)), nil
}

// GetIssuedAt expone el `iat` del claim (ACC-E02 T8i): el gate de revocación
// lo coteja contra las marcas de alcance user de revoke-all. Un token legacy
// sin iat reporta 0 (fail-closed en el gate).
func (c JWTClaims) GetIssuedAt() (*jwt.NumericDate, error) {
	if c.IssuedAt == 0 {
		return nil, nil
	}
	return jwt.NewNumericDate(time.Unix(c.IssuedAt, 0)), nil
}
func (c JWTClaims) GetNotBefore() (*jwt.NumericDate, error) { return nil, nil }

func (c JWTClaims) GetIssuer() (string, error) {
	return c.Issuer, nil
}

func (c JWTClaims) GetSubject() (string, error) {
	return c.Email, nil
}

func (c JWTClaims) GetAudience() (jwt.ClaimStrings, error) {
	return nil, nil
}

func (c JWTClaims) GetID() (string, error) {
	return c.JTI.String(), nil
}
