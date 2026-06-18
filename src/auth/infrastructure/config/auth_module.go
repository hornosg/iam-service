package config

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"iam/src/auth/application/usecase"
	"iam/src/auth/domain/port"
	"iam/src/auth/infrastructure/adapter"
	"iam/src/auth/infrastructure/controller"
	authlogging "iam/src/auth/infrastructure/logging"
	authmw "iam/src/auth/infrastructure/middleware"
	"iam/src/auth/infrastructure/persistence/repository"
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
}

// NewAuthModuleConfigFromEnv crea la configuración leyendo variables de entorno y valida seguridad.
// En producción hace log.Fatal si JWT_SECRET es inseguro; en desarrollo solo muestra warning.
func NewAuthModuleConfigFromEnv() AuthModuleConfig {
	jwtSecret := os.Getenv("JWT_SECRET")

	if err := ValidateJWTSecret(jwtSecret); err != nil {
		ginMode := os.Getenv("GIN_MODE")
		if ginMode == "release" {
			log.Fatalf("SECURITY: %v", err)
		}
		log.Printf("SECURITY WARNING: %v (allowed in development mode)", err)
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

// SetupAuthModule configura e inicializa el módulo de autenticación
func SetupAuthModule(router *gin.RouterGroup, db *sql.DB, userService port.UserService, tenantService port.TenantService, config AuthModuleConfig) {
	// Crear configuración para casos de uso
	authConfig := usecase.AuthConfig{
		AccessTokenExpiry:  config.AccessTokenExpiry,
		RefreshTokenExpiry: config.RefreshTokenExpiry,
		Namespace:          config.Namespace,
	}

	// Instanciar repositorio
	authRepo := repository.NewPostgresAuthRepository(db)

	// Instanciar logger de seguridad compartido
	securityLogger := sharedlog.NewSecurityLogger("iam")

	// Instanciar adapters
	jwtService := adapter.NewJWTServiceAdapter(config.JWTSecret)
	googleVerifier := adapter.NewHTTPGoogleTokenVerifier(config.GoogleClientID)
	// RoleResolver: resuelve slug+permisos del rol en la emisión (login/refresh) para
	// poblar los claims `roles`/`perms`. Lee la tabla roles directo (aislamiento de tipos).
	roleResolver := adapter.NewSQLRoleResolverAdapter(db)
	// PlanResolver: resuelve el tier del plan del tenant para el claim `plan` (rate limiting
	// por plan, ADR-003). Lee tenants JOIN plans directo (aislamiento de tipos).
	planResolver := adapter.NewSQLPlanResolverAdapter(db)

	// Instanciar casos de uso
	loginUseCase := usecase.NewLoginUseCase(authConfig, authRepo, userService, tenantService, jwtService, roleResolver, planResolver, googleVerifier, securityLogger)
	refreshTokenUseCase := usecase.NewRefreshTokenUseCase(authConfig, authRepo, userService, tenantService, jwtService, roleResolver, planResolver)
	validateTokenUseCase := usecase.NewValidateTokenUseCase(jwtService)
	logoutUseCase := usecase.NewLogoutUseCase(authRepo, securityLogger)
	revokeAllUseCase := usecase.NewRevokeAllUseCase(authRepo, config.AccessTokenExpiry, securityLogger)

	// Instanciar controlador
	authHandler := controller.NewAuthHandler(
		loginUseCase,
		refreshTokenUseCase,
		validateTokenUseCase,
		logoutUseCase,
		revokeAllUseCase,
	)

	// Registrar middleware de revocación de tokens
	router.Use(authmw.TokenRevocationCheck(authmw.TokenRevocationConfig{
		JWTSecret: config.JWTSecret,
		AuthRepo:  authRepo,
		ExcludedRoutes: []string{
			"/api/v1/auth/login",
			"/api/v1/auth/refresh",
			"/api/v1/auth/validate",
		},
	}))

	// Registrar rutas
	authHandler.RegisterRoutes(router)

	// Iniciar goroutine de limpieza de tokens revocados expirados
	tokenMaintenanceLogger := authlogging.NewTokenMaintenanceLogger("iam")
	go startRevocationCleanup(authRepo, tokenMaintenanceLogger)
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
