package usecase

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"

	"iam/src/auth/domain/value_object"
)

type ValidateTokenUseCase struct {
	config AuthConfig
}

func NewValidateTokenUseCase(config AuthConfig) *ValidateTokenUseCase {
	return &ValidateTokenUseCase{
		config: config,
	}
}

func (uc *ValidateTokenUseCase) Execute(tokenString string) (*value_object.TokenClaims, error) {
	// Parsear el token
	token, err := jwt.ParseWithClaims(tokenString, &value_object.TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Verificar el método de firma
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("método de firma inesperado: %v", token.Header["alg"])
		}
		return []byte(uc.config.JWTSecret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("error parseando token: %w", err)
	}

	// Verificar que el token sea válido
	if !token.Valid {
		return nil, errors.New("token inválido")
	}

	// Extraer los claims
	claims, ok := token.Claims.(*value_object.TokenClaims)
	if !ok {
		return nil, errors.New("claims inválidos")
	}

	return claims, nil
}
