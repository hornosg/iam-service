package config

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/hornosg/iam-service/src/identity/application/usecase"
	"github.com/hornosg/iam-service/src/identity/domain/port"
	"github.com/hornosg/iam-service/src/identity/infrastructure/adapter"
	"github.com/hornosg/iam-service/src/identity/infrastructure/controller"
	authlogging "github.com/hornosg/iam-service/src/identity/infrastructure/logging"
	authmw "github.com/hornosg/iam-service/src/identity/infrastructure/middleware"
	"github.com/hornosg/iam-service/src/identity/infrastructure/persistence/repository"
	sharedlog "github.com/hornosg/go-shared/infrastructure/logging"
)

const (
	insecureDefaultSecret = "your-super-secret-jwt-key"
	minJWTSecretLength    = 32
)

// AuthModuleConfig contiene la configuración para el módulo de autenticación
type AuthModuleConfig struct {
	JWTSecret          string
	AccessTokenExpiry  time.Duration
	RefreshTokenExpiry time.Duration
	Namespace          string
	GoogleClientID     string // usado solo para construir el adapter HTTP; no llega al dominio
	// SigningKey es el par RSA + kid de ACC-E03 T2 (ADR-003 §b). Nil mientras
	// T3 no consuma la firma asimétrica; en GIN_MODE=release la validación de
	// arranque es fatal si falta (mismo patrón que JWT_SECRET).
	SigningKey *SigningKey
}

// NewAuthModuleConfigFromEnv crea la configuración leyendo variables de entorno y valida seguridad.
// En producción hace log.Fatal si JWT_SECRET es inseguro; en desarrollo solo muestra warning.
// ACC-E03 T2: la clave de firma asimétrica sigue el MISMO patrón (fatal en release,
// warning en desarrollo) vía LoadSigningKeyFromEnv.
func NewAuthModuleConfigFromEnv() AuthModuleConfig {
	jwtSecret := os.Getenv("JWT_SECRET")

	ginMode := os.Getenv("GIN_MODE")

	if err := ValidateJWTSecret(jwtSecret); err != nil {
		if ginMode == "release" {
			log.Fatalf("SECURITY: %v", err)
		}
		log.Printf("SECURITY WARNING: %v (allowed in development mode)", err)
	}

	signingKey, err := LoadSigningKeyFromEnv()
	if err != nil {
		if ginMode == "release" {
			log.Fatalf("SECURITY: %v", err)
		}
		log.Printf("SECURITY WARNING: %v (allowed in development mode — HS256 seguirá firmando hasta ACC-E03 T3)", err)
	}

	googleClientID := os.Getenv("GOOGLE_CLIENT_ID")

	namespace := os.Getenv("SERVICE_NAMESPACE")
	if namespace == "" {
		namespace = "mc"
	}

	return AuthModuleConfig{
		JWTSecret:          jwtSecret,
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		Namespace:          namespace,
		GoogleClientID:     googleClientID,
		SigningKey:         signingKey,
	}
}

// ValidateJWTSecret valida que el secret sea seguro para producción.
func ValidateJWTSecret(secret string) error {
	if secret == "" {
		return fmt.Errorf("JWT_SECRET must not be empty — set a secure value via environment variable")
	}
	if secret == insecureDefaultSecret {
		return fmt.Errorf("JWT_SECRET must be changed from default value — set a secure value via environment variable")
	}
	if len(secret) < minJWTSecretLength {
		return fmt.Errorf("JWT_SECRET must be at least %d characters (got %d)", minJWTSecretLength, len(secret))
	}
	return nil
}


// SetupTokenRevocationGate registra el middleware de revocación de tokens sobre
// el grupo padre ANTES de que main cree los grupos de rutas de gestión.
//
// ACC-E02 T8g (criterio (a)): Gin congela la cadena de handlers de un grupo en
// el momento de crearlo — adminGroup/tenantScopedGroup se creaban ANTES del
// `router.Use(TokenRevocationCheck(...))` de SetupAuthModule, así que un JTI
// revocado seguía autorizando /users, /tenants/:id, /roles y /plans hasta que
// el access token expirara por tiempo (≤15 min). El logout y el revoke-all no
// cortaban el acceso a esas rutas. La corrección es de orden de registro, no de
// lógica: el gate va acá, ANTES de crear los grupos; SetupAuthModule lo recibe
// ya registrado y NO lo vuelve a registrar (un segundo Use duplicaría la
// consulta de revocación en cada request).
//
// Devuelve el repositorio de auth sobre account_app para que SetupAuthModule
// reutilice esa MISMA instancia en sus casos de uso.
func SetupTokenRevocationGate(
	router *gin.RouterGroup,
	appDB *sql.DB,
	loginDB *sql.DB,
	jwtSecret string,
) port.AuthRepository {
	authRepoApp := repository.NewPostgresAuthRepository(appDB)

	// ACC-E02 T8g (criterio (B) del gate de T8f): el resolver corre sobre el
	// pool de login — la policy users_login_lookup (019) le permite el SELECT
	// sin filtro de tenant y el grant de 017 cubre tenant_id. Deriva el tenant
	// de tokens legacy sin claim de tenant a partir del user_id verificado.
	tenantResolver := adapter.NewPostgresTenantResolver(loginDB)

	router.Use(authmw.TokenRevocationCheck(authmw.TokenRevocationConfig{
		JWTSecret:      jwtSecret,
		AuthRepo:       authRepoApp,
		TenantResolver: tenantResolver,
		ExcludedRoutes: []string{
			"/api/v1/auth/login",
			"/api/v1/auth/refresh",
		},
	}))

	return authRepoApp
}

// SetupAuthModule configura e inicializa el módulo de autenticación.
// ACC-E02 T5: dos pools de DB — appDB (account_app, RLS) para toda operación
// post-auth y loginDB (iam_login) para la fase pre-auth de credenciales.
// ACC-E02 T8g: el gate de revocación YA se registró sobre el grupo padre vía
// SetupTokenRevocationGate (antes de crear los grupos de gestión); acá llega la
// instancia de authRepoApp que ese gate usa, para que casos de uso y gate
// compartan el mismo repositorio.
func SetupAuthModule(
	router *gin.RouterGroup,
	appDB *sql.DB,
	loginDB *sql.DB,
	authRepoApp port.AuthRepository,
	userService port.UserService,
	loginUserService port.UserService,
	tenantService port.TenantService,
	config AuthModuleConfig,
) {
	// Crear configuración para casos de uso
	authConfig := usecase.AuthConfig{
		AccessTokenExpiry:  config.AccessTokenExpiry,
		RefreshTokenExpiry: config.RefreshTokenExpiry,
		Namespace:          config.Namespace,
	}

	// Repositorio de login: sólo la fase pre-auth de POST /auth/login. El de
	// app lo aporta SetupTokenRevocationGate (misma instancia que el gate).
	authRepoLogin := repository.NewPostgresAuthRepository(loginDB)

	// Instanciar logger de seguridad compartido
	securityLogger := sharedlog.NewSecurityLogger("iam")

	// Instanciar adapters (todos sobre account_app; el tenant ya se conoce post-auth).
	jwtService := adapter.NewJWTServiceAdapter(config.JWTSecret)
	googleVerifier := adapter.NewHTTPGoogleTokenVerifier(config.GoogleClientID)
	roleResolver := adapter.NewSQLRoleResolverAdapter(appDB)
	planResolver := adapter.NewSQLPlanResolverAdapter(appDB)

	// Instanciar casos de uso
	loginUseCase := usecase.NewLoginUseCase(
		authConfig,
		authRepoLogin,
		authRepoApp,
		loginUserService,
		tenantService,
		jwtService,
		roleResolver,
		planResolver,
		googleVerifier,
		securityLogger,
	)
	refreshTokenUseCase := usecase.NewRefreshTokenUseCase(authConfig, authRepoApp, userService, tenantService, jwtService, roleResolver, planResolver)
	logoutUseCase := usecase.NewLogoutUseCase(authRepoApp, securityLogger)
	revokeAllUseCase := usecase.NewRevokeAllUseCase(authRepoApp, config.AccessTokenExpiry, securityLogger)

	// Instanciar controlador
	authHandler := controller.NewAuthHandler(
		loginUseCase,
		refreshTokenUseCase,
		logoutUseCase,
		revokeAllUseCase,
	)

	// El middleware de revocación NO se registra acá: SetupTokenRevocationGate
	// lo registró sobre el grupo padre ANTES de que main creara los grupos de
	// gestión (ACC-E02 T8g, criterio (a) — Gin congela la cadena al crear el
	// grupo). authRepoApp ya llegó compartido desde ahí.

	// Registrar rutas
	authHandler.RegisterRoutes(router)

	// Iniciar goroutine de limpieza de tokens revocados expirados
	tokenMaintenanceLogger := authlogging.NewTokenMaintenanceLogger("iam")
	go startRevocationCleanup(authRepoApp, tokenMaintenanceLogger)
}

func startRevocationCleanup(repo port.AuthRepository, logger port.TokenMaintenanceEventLogger) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		count, err := repo.CleanupExpiredRevocations(ctx)
		cancel()
		if err != nil {
			logger.Log(port.TokenMaintenanceEvent{
				Event:  port.EventRevocationCleanupFailed,
				Reason: err.Error(),
			})
		} else if count > 0 {
			logger.Log(port.TokenMaintenanceEvent{
				Event: port.EventRevocationCleanupCompleted,
				Count: count,
			})
		}
	}
}
