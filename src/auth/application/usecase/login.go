package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"iam/src/auth/application/request"
	"iam/src/auth/application/response"
	"iam/src/auth/domain/entity"
	"iam/src/auth/domain/port"
	"iam/src/auth/domain/value_object"
	sharedport "github.com/hornosg/go-shared/domain/port"
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
	Namespace          string
}

type LoginUseCase struct {
	config              AuthConfig
	authRepo            port.AuthRepository
	userService         port.UserService
	tenantService       port.TenantService
	jwtService          port.JWTService
	roleResolver        port.RoleResolver
	planResolver        port.PlanResolver
	googleTokenVerifier port.GoogleTokenVerifier
	securityLogger      sharedport.SecurityEventLogger
}

func NewLoginUseCase(
	config AuthConfig,
	authRepo port.AuthRepository,
	userService port.UserService,
	tenantService port.TenantService,
	jwtService port.JWTService,
	roleResolver port.RoleResolver,
	planResolver port.PlanResolver,
	googleTokenVerifier port.GoogleTokenVerifier,
	securityLogger sharedport.SecurityEventLogger,
) *LoginUseCase {
	return &LoginUseCase{
		config:              config,
		authRepo:            authRepo,
		userService:         userService,
		tenantService:       tenantService,
		jwtService:          jwtService,
		roleResolver:        roleResolver,
		planResolver:        planResolver,
		googleTokenVerifier: googleTokenVerifier,
		securityLogger:      securityLogger,
	}
}

func (uc *LoginUseCase) Execute(ctx context.Context, req *request.LoginRequest) (*response.LoginResponse, error) {
	return uc.ExecuteWithInfo(ctx, req, "", "")
}

func (uc *LoginUseCase) ExecuteWithInfo(ctx context.Context, req *request.LoginRequest, ipAddress, userAgent string) (*response.LoginResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	var user *port.UserData
	var err error

	switch req.Provider {
	case value_object.LocalAuth:
		user, err = uc.loginLocal(ctx, req)
	case value_object.GoogleAuth:
		user, err = uc.loginGoogle(ctx, req)
	default:
		return nil, fmt.Errorf("proveedor de autenticación no soportado: %s", req.Provider)
	}

	if err != nil {
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

	uc.securityLogger.Log(sharedport.SecurityEvent{
		Event:     sharedport.EventLoginSuccess,
		UserID:    user.ID.String(),
		TenantID:  user.TenantID.String(),
		Email:     user.Email,
		IPAddress: ipAddress,
		UserAgent: userAgent,
	})

	// Generar tokens
	accessToken, err := uc.generateAccessToken(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("error generando access token: %w", err)
	}

	refreshToken, err := uc.generateRefreshToken(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("error generando refresh token: %w", err)
	}

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
	user, err := uc.userService.FindUserByEmail(ctx, req.Email, req.TenantID)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if value_object.AuthProvider(user.Provider) != value_object.LocalAuth {
		return nil, fmt.Errorf("este usuario usa autenticación %s", user.Provider)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	// Validar tenant si se proporcionó
	if req.TenantID != nil && *req.TenantID != user.TenantID {
		return nil, ErrInvalidCredentials
	}

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

func (uc *LoginUseCase) generateAccessToken(ctx context.Context, user *port.UserData) (string, error) {
	features, err := uc.tenantService.Execute(ctx, user.TenantID)
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

	roles, perms := resolveRoleClaims(ctx, uc.roleResolver, user.RoleID)
	claims.Roles = roles
	claims.Perms = perms
	claims.Plan = resolvePlanClaim(ctx, uc.planResolver, user.TenantID)

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
