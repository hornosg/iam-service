package usecase

import (
	"iam/src/auth/domain/port"
	"iam/src/auth/domain/value_object"
)

type ValidateTokenUseCase struct {
	jwtService port.JWTService
}

func NewValidateTokenUseCase(jwtService port.JWTService) *ValidateTokenUseCase {
	return &ValidateTokenUseCase{jwtService: jwtService}
}

func (uc *ValidateTokenUseCase) Execute(tokenString string) (*value_object.TokenClaims, error) {
	return uc.jwtService.Parse(tokenString)
}
