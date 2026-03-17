package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"iam/src/auth/application/request"
	"iam/src/auth/application/response"
	"iam/src/auth/domain/entity"
	"iam/src/auth/domain/port"
	"iam/src/auth/domain/value_object"
	"iam/src/auth/infrastructure/logging"
	tenant_vo "iam/src/tenant/domain/value_object"
)

var (
	ErrInvalidCredentials = errors.New("credenciales inválidas")
	ErrUserNotFound       = errors.New("usuario no encontrado")
	ErrInvalidToken       = errors.New("token inválido")
	ErrExpiredToken       = errors.New("token expirado")
)

type AuthConfig struct {
	JWTSecret          string
	AccessTokenExpiry  time.Duration
	RefreshTokenExpiry time.Duration
	GoogleClientID     string
}

type LoginUseCase struct {
	config         AuthConfig
	authRepo       port.AuthRepository
	userService    port.UserService
	tenantService  port.TenantService
	httpClient     *http.Client
	securityLogger *logging.SecurityLogger
}

func NewLoginUseCase(
	config AuthConfig,
	authRepo port.AuthRepository,
	userService port.UserService,
	tenantService port.TenantService,
) *LoginUseCase {
	return &LoginUseCase{
		config:         config,
		authRepo:       authRepo,
		userService:    userService,
		tenantService:  tenantService,
		httpClient:     &http.Client{},
		securityLogger: logging.NewSecurityLogger(),
	}
}

func (uc *LoginUseCase) Execute(ctx context.Context, req *request.LoginRequest) (*response.LoginResponse, error) {
	return uc.ExecuteWithInfo(ctx, req, "", "")
}

func (uc *LoginUseCase) ExecuteWithInfo(ctx context.Context, req *request.LoginRequest, ipAddress, userAgent string) (*response.LoginResponse, error) {
	log.Printf("[LOGIN] Iniciando proceso de login para email: %s, provider: %s", req.Email, req.Provider)

	if err := req.Validate(); err != nil {
		log.Printf("[LOGIN] Error de validación de request: %v", err)
		return nil, err
	}

	var user *port.UserData
	var err error

	switch req.Provider {
	case value_object.LocalAuth:
		log.Printf("[LOGIN] Procesando autenticación local para: %s", req.Email)
		user, err = uc.loginLocal(ctx, req)
	case value_object.GoogleAuth:
		log.Printf("[LOGIN] Procesando autenticación Google para: %s", req.Email)
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
		uc.securityLogger.LogLoginFailed(req.Email, ipAddress, userAgent, reason)
		return nil, err
	}

	log.Printf("[LOGIN] Autenticación exitosa para usuario ID: %s, email: %s", user.ID, user.Email)
	uc.securityLogger.LogLoginSuccess(user.ID.String(), user.TenantID.String(), user.Email, ipAddress, userAgent)

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

	log.Printf("[LOGIN] Tokens generados exitosamente para usuario: %s", user.Email)

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
	log.Printf("[LOGIN_LOCAL] Buscando usuario por email: %s", req.Email)

	var tenantIDLog string
	if req.TenantID != nil {
		tenantIDLog = req.TenantID.String()
	} else {
		tenantIDLog = "nil"
	}
	log.Printf("[LOGIN_LOCAL] Tenant ID en request: %s", tenantIDLog)

	user, err := uc.userService.FindUserByEmail(ctx, req.Email, req.TenantID)
	if err != nil {
		log.Printf("[LOGIN_LOCAL] Usuario no encontrado para email: %s, error: %v", req.Email, err)
		return nil, ErrInvalidCredentials
	}

	log.Printf("[LOGIN_LOCAL] Usuario encontrado - ID: %s, Email: %s, Provider: %s, Status: %s, TenantID: %s",
		user.ID, user.Email, user.Provider, user.Status, user.TenantID)

	if value_object.AuthProvider(user.Provider) != value_object.LocalAuth {
		log.Printf("[LOGIN_LOCAL] Usuario usa proveedor diferente: %s (esperado: LOCAL)", user.Provider)
		return nil, fmt.Errorf("este usuario usa autenticación %s", user.Provider)
	}

	log.Printf("[LOGIN_LOCAL] Verificando password para usuario: %s", user.Email)
	log.Printf("[LOGIN_LOCAL] Password hash en BD: %s", user.PasswordHash)
	log.Printf("[LOGIN_LOCAL] Password recibida: %s", req.Password)

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		log.Printf("[LOGIN_LOCAL] Password no coincide para usuario: %s, error bcrypt: %v", user.Email, err)
		return nil, ErrInvalidCredentials
	}

	log.Printf("[LOGIN_LOCAL] Password verificada correctamente para usuario: %s", user.Email)

	// Validar tenant si se proporcionó
	if req.TenantID != nil && *req.TenantID != user.TenantID {
		log.Printf("[LOGIN_LOCAL] Tenant ID no coincide - Request: %s, Usuario: %s", *req.TenantID, user.TenantID)
		return nil, ErrInvalidCredentials
	}

	log.Printf("[LOGIN_LOCAL] Autenticación local exitosa para usuario: %s", user.Email)
	return user, nil
}

func (uc *LoginUseCase) loginGoogle(ctx context.Context, req *request.LoginRequest) (*port.UserData, error) {
	// Verificar token de Google
	resp, err := uc.httpClient.Get("https://oauth2.googleapis.com/tokeninfo?id_token=" + req.GoogleToken)
	if err != nil {
		return nil, fmt.Errorf("error verificando token de Google: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("token de Google inválido")
	}

	var tokenInfo struct {
		Aud   string `json:"aud"`
		Sub   string `json:"sub"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenInfo); err != nil {
		return nil, fmt.Errorf("error decodificando respuesta de Google: %w", err)
	}

	if tokenInfo.Aud != uc.config.GoogleClientID {
		return nil, errors.New("client ID de Google inválido")
	}

	// Buscar usuario por ID federado
	user, err := uc.authRepo.GetUserByFederatedID(ctx, value_object.GoogleAuth, tokenInfo.Sub, req.TenantID)
	if err == nil {
		if req.TenantID != nil && *req.TenantID != user.TenantID {
			return nil, ErrInvalidCredentials
		}
		return &user, nil
	}

	// Si no existe, buscar por email
	user2, err := uc.userService.FindUserByEmail(ctx, tokenInfo.Email, req.TenantID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	if req.TenantID != nil && *req.TenantID != user2.TenantID {
		return nil, ErrInvalidCredentials
	}

	// Vincular ID federado
	if err := uc.authRepo.LinkFederatedID(ctx, user2.ID, value_object.GoogleAuth, tokenInfo.Sub); err != nil {
		return nil, fmt.Errorf("error vinculando ID federado: %w", err)
	}

	return user2, nil
}

func (uc *LoginUseCase) generateAccessToken(user *port.UserData) (string, error) {
	// Obtener features del tenant
	features, err := uc.tenantService.Execute(context.Background(), user.TenantID)
	if err != nil {
		// Si no se pueden obtener las features, usar valores por defecto
		features = &tenant_vo.TenantFeatures{
			FriendsFamily:    false,
			PremiumAnalytics: false,
		}
	}

	claims := value_object.NewTokenClaims(
		user.ID,
		user.TenantID,
		user.RoleID,
		user.Email,
		features,
		time.Now().Add(uc.config.AccessTokenExpiry),
	)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(uc.config.JWTSecret))
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
