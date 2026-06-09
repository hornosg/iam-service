package port

import "iam/src/auth/domain/value_object"

type JWTService interface {
	Sign(claims *value_object.TokenClaims) (string, error)
	Parse(tokenString string) (*value_object.TokenClaims, error)
}
