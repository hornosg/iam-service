package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"time"

	"iam/src/auth/application/response"
	"iam/src/auth/domain/entity"
	"iam/src/auth/domain/port"
	"iam/src/auth/domain/value_object"
)

type RefreshTokenUseCase struct {
	config        AuthConfig
	authRepo      port.AuthRepository
	userService   port.UserService
	tenantService port.TenantService
	jwtService    port.JWTService
}

func NewRefreshTokenUseCase(
	config AuthConfig,
	authRepo port.AuthRepository,
	userService port.UserService,
	tenantService port.TenantService,
	jwtService port.JWTService,
) *RefreshTokenUseCase {
	return &RefreshTokenUseCase{
		config:        config,
		authRepo:      authRepo,
		userService:   userService,
		tenantService: tenantService,
		jwtService:    jwtService,
	}
}

func (uc *RefreshTokenUseCase) Execute(ctx context.Context, refreshToken string) (*response.LoginResponse, error) {
	// Obtener refresh token de la base de datos
	token, err := uc.authRepo.GetRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, ErrInvalidToken
	}

	if token.IsExpired() {
		// Eliminar token expirado
		_ = uc.authRepo.DeleteRefreshToken(ctx, refreshToken)
		return nil, ErrExpiredToken
	}

	// Obtener información actualizada del usuario
	user, err := uc.userService.FindUserByID(ctx, token.UserID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// Generar nuevo access token
	accessToken, err := uc.generateAccessToken(user)
	if err != nil {
		return nil, err
	}

	// Generar nuevo refresh token y eliminar el anterior
	newRefreshToken, err := uc.generateRefreshToken(ctx, user)
	if err != nil {
		return nil, err
	}

	// Eliminar el refresh token anterior
	_ = uc.authRepo.DeleteRefreshToken(ctx, refreshToken)

	userData := response.UserData{
		ID:       user.ID,
		Email:    user.Email,
		TenantID: user.TenantID,
		RoleID:   user.RoleID,
		Status:   user.Status,
	}

	return response.NewLoginResponse(accessToken, newRefreshToken, int(uc.config.AccessTokenExpiry.Seconds()), userData), nil
}

func (uc *RefreshTokenUseCase) generateAccessToken(user *port.UserData) (string, error) {
	features, err := uc.tenantService.Execute(context.Background(), user.TenantID)
	if err != nil {
		features = value_object.DefaultTenantFeatures()
	}

	claims := value_object.NewTokenClaims(
		user.ID,
		user.TenantID,
		user.RoleID,
		user.Email,
		uc.config.Namespace,
		features,
		time.Now().Add(uc.config.AccessTokenExpiry),
	)

	return uc.jwtService.Sign(claims)
}

func (uc *RefreshTokenUseCase) generateRefreshToken(ctx context.Context, user *port.UserData) (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	refreshToken := entity.NewRefreshToken(
		user.ID,
		token,
		time.Now().Add(uc.config.RefreshTokenExpiry),
	)

	if err := uc.authRepo.CreateRefreshToken(ctx, refreshToken); err != nil {
		return "", err
	}

	return token, nil
}
