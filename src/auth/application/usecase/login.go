package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"iam/src/auth/application/request"
	"iam/src/auth/application/response"
	"iam/src/auth/domain/entity"
	"iam/src/auth/domain/port"
	"iam/src/auth/domain/value_object"
	sharedctx "iam/src/shared/context"
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
	preAuthAuthRepo     port.AuthRepository
	postAuthAuthRepo    port.AuthRepository
	preAuthUserService  port.UserService
	tenantService       port.TenantService
	jwtService          port.JWTService
	roleResolver        port.RoleResolver
	planResolver        port.PlanResolver
	googleTokenVerifier port.GoogleTokenVerifier
	securityLogger      sharedport.SecurityEventLogger
}

func NewLoginUseCase(
	config AuthConfig,
	preAuthAuthRepo port.AuthRepository,
	postAuthAuthRepo port.AuthRepository,
	preAuthUserService port.UserService,
	tenantService port.TenantService,
	jwtService port.JWTService,
	roleResolver port.RoleResolver,
	planResolver port.PlanResolver,
	googleTokenVerifier port.GoogleTokenVerifier,
	securityLogger sharedport.SecurityEventLogger,
) *LoginUseCase {
	return &LoginUseCase{
		config:              config,
		preAuthAuthRepo:     preAuthAuthRepo,
		postAuthAuthRepo:    postAuthAuthRepo,
		preAuthUserService:  preAuthUserService,
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
	var link *federatedLinkRequest
	var err error

	switch req.Provider {
	case value_object.LocalAuth:
		user, err = uc.loginLocal(ctx, req)
	case value_object.GoogleAuth:
		user, link, err = uc.loginGoogle(ctx, req)
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

	// A partir de acá el tenant YA se conoce: toda operación post-auth corre
	// bajo account_app con el contexto RLS de ese tenant.
	ctx = sharedctx.WithTenantID(ctx, user.TenantID)

	// Generar tokens (post-auth, account_app + RLS).
	accessToken, err := uc.generateAccessToken(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("error generando access token: %w", err)
	}

	refreshToken, err := uc.generateRefreshToken(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("error generando refresh token: %w", err)
	}

	// Vincular ID federado post-auth (T1-D1, T2 carry-forward): nunca bajo iam_login.
	if link != nil {
		if err := uc.postAuthAuthRepo.LinkFederatedID(ctx, link.userID, link.provider, link.federatedID); err != nil {
			return nil, fmt.Errorf("error vinculando ID federado: %w", err)
		}
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
	user, err := uc.preAuthUserService.FindUserByEmail(ctx, req.Email, req.TenantID)
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

// federatedLinkRequest describe un linkeo de identidad federada pendiente de
// ejecutar post-auth, bajo account_app con RLS (T1-D1, T2 carry-forward).
type federatedLinkRequest struct {
	userID      uuid.UUID
	provider    value_object.AuthProvider
	federatedID string
}

func (uc *LoginUseCase) loginGoogle(ctx context.Context, req *request.LoginRequest) (*port.UserData, *federatedLinkRequest, error) {
	claims, err := uc.googleTokenVerifier.Verify(ctx, req.GoogleToken)
	if err != nil {
		return nil, nil, err
	}

	// Buscar usuario por ID federado (pre-auth, iam_login).
	user, err := uc.preAuthAuthRepo.GetUserByFederatedID(ctx, value_object.GoogleAuth, claims.Sub, req.TenantID)
	if err == nil {
		if req.TenantID != nil && *req.TenantID != user.TenantID {
			return nil, nil, ErrInvalidCredentials
		}
		return &user, nil, nil
	}

	// Si no existe, buscar por email (pre-auth, iam_login).
	user2, err := uc.preAuthUserService.FindUserByEmail(ctx, claims.Email, req.TenantID)
	if err != nil {
		return nil, nil, ErrUserNotFound
	}

	if req.TenantID != nil && *req.TenantID != user2.TenantID {
		return nil, nil, ErrInvalidCredentials
	}

	// El linkeo se ejecuta post-auth bajo account_app (T1-D1).
	link := &federatedLinkRequest{
		userID:      user2.ID,
		provider:    value_object.GoogleAuth,
		federatedID: claims.Sub,
	}
	return user2, link, nil
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

	if err := uc.postAuthAuthRepo.CreateRefreshToken(ctx, refreshToken); err != nil {
		return "", err
	}

	return token, nil
}
