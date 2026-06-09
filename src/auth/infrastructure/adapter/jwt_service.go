package adapter

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"

	"iam/src/auth/domain/value_object"
)

type JWTServiceAdapter struct {
	secret string
}

func NewJWTServiceAdapter(secret string) *JWTServiceAdapter {
	return &JWTServiceAdapter{secret: secret}
}

func (s *JWTServiceAdapter) Sign(claims *value_object.TokenClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, JWTClaims{TokenClaims: *claims})
	return token.SignedString([]byte(s.secret))
}

func (s *JWTServiceAdapter) Parse(tokenString string) (*value_object.TokenClaims, error) {
	jwtClaims := &JWTClaims{}
	token, err := jwt.ParseWithClaims(tokenString, jwtClaims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("método de firma inesperado: %v", t.Header["alg"])
		}
		return []byte(s.secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("error parseando token: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("token inválido")
	}
	tc := jwtClaims.TokenClaims
	return &tc, nil
}
