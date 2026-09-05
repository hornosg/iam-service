package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/hornosg/iam-service/src/identity/application/response"
	"github.com/hornosg/iam-service/src/identity/domain/entity"
	"github.com/hornosg/iam-service/src/identity/domain/port"
	"github.com/hornosg/iam-service/src/identity/domain/value_object"
	sharedctx "github.com/hornosg/iam-service/src/shared/context"
)

type RefreshTokenUseCase struct {
	config        AuthConfig
	authRepo      port.AuthRepository
	userService   port.UserService
	tenantService port.TenantService
	jwtService    port.JWTService
	roleResolver  port.RoleResolver
	planResolver  port.PlanResolver
}

func NewRefreshTokenUseCase(
	config AuthConfig,
	authRepo port.AuthRepository,
	userService port.UserService,
	tenantService port.TenantService,
	jwtService port.JWTService,
	roleResolver port.RoleResolver,
	planResolver port.PlanResolver,
) *RefreshTokenUseCase {
	return &RefreshTokenUseCase{
		config:        config,
		authRepo:      authRepo,
		userService:   userService,
		tenantService: tenantService,
		jwtService:    jwtService,
		roleResolver:  roleResolver,
		planResolver:  planResolver,
	}
}

func (uc *RefreshTokenUseCase) Execute(ctx context.Context, refreshToken string) (*response.LoginResponse, error) {
	return uc.ExecuteWithPresenter(ctx, refreshToken, "")
}

// ExecuteWithPresenter ejecuta el refresh admitiendo un access token Bearer
// OPCIONAL. ACC-E02 T8e: el endpoint no exige Bearer (contrato vigente con los
// consumidores), pero si el cliente lo manda —típicamente expirado— el usecase
// verifica su FIRMA y cruza el tenant reclamado contra el tenant resuelto del
// refresh token: si difieren, el par (access, refresh) es inconsistente y el
// refresh se rechaza. Es defensa en profundidad contra uso cross-tenant de un
// refresh token robado, no el control primario (la posesión del refresh token
// sigue siendo la credencial, igual que en cualquier esquema bearer).
func (uc *RefreshTokenUseCase) ExecuteWithPresenter(ctx context.Context, refreshToken, presenterToken string) (*response.LoginResponse, error) {
	// Obtener refresh token de la base de datos. Paso PRE-auth: corre con el
	// escape de presentación (app.refresh_digest) y devuelve la fila con su
	// tenant denormalizado — acá recién se conoce el tenant.
	token, err := uc.authRepo.GetRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, ErrInvalidToken
	}

	if token.TenantID == uuid.Nil {
		// Inalcanzable con el repo real (tenant_id NOT NULL desde la 021); un
		// mock o una fila sin denormalizar no deben poder abrir un contexto sin
		// tenant: la RLS fallaría cerrada más abajo de todos modos.
		return nil, ErrInvalidToken
	}

	// Cross-check de coherencia con el Bearer presentado (ver arriba).
	if presenterToken != "" {
		if claims, perr := uc.jwtService.ParseIgnoringExpiry(presenterToken); perr == nil && claims.TenantID != uuid.Nil {
			if claims.TenantID != token.TenantID {
				return nil, ErrInvalidToken
			}
		}
	}

	// A partir de acá todo corre bajo la RLS del tenant dueño del token.
	ctx = sharedctx.WithTenantID(ctx, token.TenantID)

	if token.IsExpired() {
		// Eliminar token expirado (bajo RLS del tenant dueño). T8f: si un
		// request concurrente ya consumió la fila, el token está muerto igual —
		// el cliente recibe expirado, no un 500 de carrera.
		if err := uc.authRepo.DeleteRefreshToken(ctx, refreshToken); err != nil && !errors.Is(err, port.ErrRefreshTokenAlreadyConsumed) {
			return nil, err
		}
		return nil, ErrExpiredToken
	}

	// Obtener información actualizada del usuario
	user, err := uc.userService.FindUserByID(ctx, token.UserID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// Generar nuevo access token (re-resuelve el rol VIGENTE → propaga cambios de rol)
	accessToken, err := uc.generateAccessToken(ctx, user)
	if err != nil {
		return nil, err
	}

	// Rotación: eliminar el refresh token ANTES de emitir el nuevo. Si la
	// eliminación falla, abortar sin crear el reemplazo — dejar el viejo vivo
	// mientras se entrega uno nuevo sería ampliar la vida de la credencial
	// (antes este error se ignoraba con `_ =`: T8e). T8f: un DELETE de 0 filas
	// ya no pasa inadvertido — la propiedad single-use exige exactamente una
	// fila borrada; si otro request concurrente la consumió, este request NO
	// recibe credenciales nuevas (credencial inválida, 401).
	if err := uc.authRepo.DeleteRefreshToken(ctx, refreshToken); err != nil {
		if errors.Is(err, port.ErrRefreshTokenAlreadyConsumed) {
			return nil, ErrInvalidToken
		}
		return nil, err
	}

	newRefreshToken, err := uc.generateRefreshToken(ctx, user)
	if err != nil {
		return nil, err
	}

	userData := response.UserData{
		ID:       user.ID,
		Email:    user.Email,
		TenantID: user.TenantID,
		RoleID:   user.RoleID,
		Status:   user.Status,
	}

	return response.NewLoginResponse(accessToken, newRefreshToken, int(uc.config.AccessTokenExpiry.Seconds()), userData), nil
}

func (uc *RefreshTokenUseCase) generateAccessToken(ctx context.Context, user *port.UserData) (string, error) {
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
