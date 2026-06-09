package adapter

import (
	"time"

	"github.com/golang-jwt/jwt/v5"

	"iam/src/auth/domain/value_object"
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

func (c JWTClaims) GetIssuedAt() (*jwt.NumericDate, error)  { return nil, nil }
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
