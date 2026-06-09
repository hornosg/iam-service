package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"

	"iam/src/auth/application/request"
	"iam/src/auth/application/response"
	"iam/src/auth/domain/entity"
	"iam/src/auth/domain/port"
	"iam/src/auth/domain/value_object"
	sharedport "github.com/mercadocercano/go-shared/domain/port"
)

var (
	ErrInvalidCredentials = errors.New("credenciales inválidas")
	ErrUserNotFound       = errors.New("usuario no encontrado")
	ErrInvalidToken       = errors.New("token inválido")
	ErrExpiredToken       = errors.New("token expirado")
)

type AuthConfig struct {
	AccessTokenExpiry  time.Duration
	RefreshTokenExpiry time.Duration
}

type LoginUseCase struct {
	config              AuthConfig
	authRepo            port.AuthRepository
	userService         port.UserService
	tenantService       port.TenantService
	jwtService          port.JWTService
	googleTokenVerifier port.GoogleTokenVerifier
	securityLogger      sharedport.SecurityEventLogger
}

func NewLoginUseCase(
	config AuthConfig,
	authRepo port.AuthRepository,
	userService port.UserService,
	tenantService port.TenantService,
	jwtService port.JWTService,
	googleTokenVerifier port.GoogleTokenVerifier,
	securityLogger sharedport.SecurityEventLogger,
) *LoginUseCase {
	return &LoginUseCase{
		config:              config,
		authRepo:            authRepo,
		userService:         userService,
		tenantService:       tenantService,
		jwtService:          jwtService,
		googleTokenVerifier: googleTokenVerifier,
		securityLogger:      securityLogger,
	}
}

func (uc *LoginUseCase) Execute(ctx context.Context, req *request.LoginRequest) (*response.LoginResponse, error) {
	return uc.ExecuteWithInfo(ctx, req, "", "")
}

func (uc *LoginUseCase) ExecuteWithInfo(ctx context.Context, req *request.LoginRequest, ipAddress, userAgent string) (*response.LoginResponse, error) {
	log.Printf("[LOGIN] Iniciando proceso de login, provider: %s", req.Provider)

	if err := req.Validate(); err != nil {
		log.Printf("[LOGIN] Error de validación de request: %v", err)
		return nil, err
	}

	var user *port.UserData
	var err error

	switch req.Provider {
	case value_object.LocalAuth:
		log.Printf("[LOGIN] Procesando autenticación local")
		user, err = uc.loginLocal(ctx, req)
	case value_object.GoogleAuth:
		log.Printf("[LOGIN] Procesando autenticación Google")
		user, err = uc.loginGoogle(ctx, req)
	default:
		log.Printf("[LOGIN] Proveedor no soportado: %s", req.Provider)
		return nil, fmt.Errorf("proveedor de autenticación no soportado: %s", req.Provider)
	}

	if err != nil {
		log.Printf("[LOGIN] Error en proceso de autenticación: %v", err)
		reason := "unknown"
		if errors.Is(err, ErrInvalidCredentials) {
			reason = "invalid_credentials"
		} else if errors.Is(err, ErrUserNotFound) {
			reason = "user_not_found"
		}
		uc.securityLogger.Log(sharedport.SecurityEvent{
			Event:     sharedport.EventLoginFailed,
			Email:     req.Email,
			IPAddress: ipAddress,
			UserAgent: userAgent,
			Reason:    reason,
		})
		return nil, err
	}

	log.Printf("[LOGIN] Autenticación exitosa para usuario ID: %s", user.ID)
	uc.securityLogger.Log(sharedport.SecurityEvent{
		Event:     sharedport.EventLoginSuccess,
		UserID:    user.ID.String(),
		TenantID:  user.TenantID.String(),
		Email:     user.Email,
		IPAddress: ipAddress,
		UserAgent: userAgent,
	})

	// Generar tokens
	accessToken, err := uc.generateAccessToken(user)
	if err != nil {
		log.Printf("[LOGIN] Error generando access token: %v", err)
		return nil, fmt.Errorf("error generando access token: %w", err)
	}

	refreshToken, err := uc.generateRefreshToken(ctx, user)
	if err != nil {
		log.Printf("[LOGIN] Error generando refresh token: %v", err)
		return nil, fmt.Errorf("error generando refresh token: %w", err)
	}

	log.Printf("[LOGIN] Tokens generados exitosamente para usuario ID: %s", user.ID)

	userData := response.UserData{
		ID:       user.ID,
		Email:    user.Email,
		TenantID: user.TenantID,
		RoleID:   user.RoleID,
		Status:   user.Status,
	}

	return response.NewLoginResponse(accessToken, refreshToken, int(uc.config.AccessTokenExpiry.Seconds()), userData), nil
}

func (uc *LoginUseCase) loginLocal(ctx context.Context, req *request.LoginRequest) (*port.UserData, error) {
	log.Printf("[LOGIN_LOCAL] Buscando usuario por credenciales")

	user, err := uc.userService.FindUserByEmail(ctx, req.Email, req.TenantID)
	if err != nil {
		log.Printf("[LOGIN_LOCAL] Usuario no encontrado")
		return nil, ErrInvalidCredentials
	}

	log.Printf("[LOGIN_LOCAL] Usuario encontrado - ID: %s, Provider: %s, Status: %s, TenantID: %s",
		user.ID, user.Provider, user.Status, user.TenantID)

	if value_object.AuthProvider(user.Provider) != value_object.LocalAuth {
		log.Printf("[LOGIN_LOCAL] Usuario usa proveedor diferente: %s (esperado: LOCAL)", user.Provider)
		return nil, fmt.Errorf("este usuario usa autenticación %s", user.Provider)
	}

	log.Printf("[LOGIN_LOCAL] Verificando password para usuario ID: %s", user.ID)

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		log.Printf("[LOGIN_LOCAL] Password no coincide para usuario ID: %s", user.ID)
		return nil, ErrInvalidCredentials
	}

	log.Printf("[LOGIN_LOCAL] Password verificada correctamente para usuario ID: %s", user.ID)

	// Validar tenant si se proporcionó
	if req.TenantID != nil && *req.TenantID != user.TenantID {
		log.Printf("[LOGIN_LOCAL] Tenant ID no coincide - Request: %s, Usuario: %s", *req.TenantID, user.TenantID)
		return nil, ErrInvalidCredentials
	}

	log.Printf("[LOGIN_LOCAL] Autenticación local exitosa para usuario ID: %s", user.ID)
	return user, nil
}

func (uc *LoginUseCase) loginGoogle(ctx context.Context, req *request.LoginRequest) (*port.UserData, error) {
	claims, err := uc.googleTokenVerifier.Verify(ctx, req.GoogleToken)
	if err != nil {
		return nil, err
	}

	// Buscar usuario por ID federado
	user, err := uc.authRepo.GetUserByFederatedID(ctx, value_object.GoogleAuth, claims.Sub, req.TenantID)
	if err == nil {
		if req.TenantID != nil && *req.TenantID != user.TenantID {
			return nil, ErrInvalidCredentials
		}
		return &user, nil
	}

	// Si no existe, buscar por email
	user2, err := uc.userService.FindUserByEmail(ctx, claims.Email, req.TenantID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	if req.TenantID != nil && *req.TenantID != user2.TenantID {
		return nil, ErrInvalidCredentials
	}

	// Vincular ID federado
	if err := uc.authRepo.LinkFederatedID(ctx, user2.ID, value_object.GoogleAuth, claims.Sub); err != nil {
		return nil, fmt.Errorf("error vinculando ID federado: %w", err)
	}

	return user2, nil
}

func (uc *LoginUseCase) generateAccessToken(user *port.UserData) (string, error) {
	features, err := uc.tenantService.Execute(context.Background(), user.TenantID)
	if err != nil {
		features = value_object.DefaultTenantFeatures()
	}

	claims := value_object.NewTokenClaims(
		user.ID,
		user.TenantID,
		user.RoleID,
		user.Email,
		features,
		time.Now().Add(uc.config.AccessTokenExpiry),
	)

	return uc.jwtService.Sign(claims)
}

func (uc *LoginUseCase) generateRefreshToken(ctx context.Context, user *port.UserData) (string, error) {
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
