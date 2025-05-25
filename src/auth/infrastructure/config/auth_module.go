package config

import (
	"database/sql"
	"time"

	"github.com/gin-gonic/gin"

	"iam/src/auth/application/usecase"
	"iam/src/auth/domain/port"
	"iam/src/auth/infrastructure/controller"
	"iam/src/auth/infrastructure/persistence/repository"
)

// AuthModuleConfig contiene la configuración para el módulo de autenticación
type AuthModuleConfig struct {
	JWTSecret          string
	AccessTokenExpiry  time.Duration
	RefreshTokenExpiry time.Duration
	GoogleClientID     string
}

// DefaultAuthModuleConfig devuelve una configuración por defecto
func DefaultAuthModuleConfig() AuthModuleConfig {
	return AuthModuleConfig{
		JWTSecret:          "your-super-secret-jwt-key", // En producción debe venir de variables de entorno
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour, // 7 días
		GoogleClientID:     "",                 // Debe configurarse desde variables de entorno
	}
}

// SetupAuthModule configura e inicializa el módulo de autenticación
func SetupAuthModule(router *gin.RouterGroup, db *sql.DB, userService port.UserService, tenantService port.TenantService, config AuthModuleConfig) {
	// Crear configuración para casos de uso
	authConfig := usecase.AuthConfig{
		JWTSecret:          config.JWTSecret,
		AccessTokenExpiry:  config.AccessTokenExpiry,
		RefreshTokenExpiry: config.RefreshTokenExpiry,
		GoogleClientID:     config.GoogleClientID,
	}

	// Instanciar repositorio
	authRepo := repository.NewPostgresAuthRepository(db)

	// Instanciar casos de uso
	loginUseCase := usecase.NewLoginUseCase(authConfig, authRepo, userService, tenantService)
	refreshTokenUseCase := usecase.NewRefreshTokenUseCase(authConfig, authRepo, userService, tenantService)
	validateTokenUseCase := usecase.NewValidateTokenUseCase(authConfig)
	logoutUseCase := usecase.NewLogoutUseCase(authRepo)

	// Instanciar controlador
	authHandler := controller.NewAuthHandler(
		loginUseCase,
		refreshTokenUseCase,
		validateTokenUseCase,
		logoutUseCase,
	)

	// Registrar rutas
	authHandler.RegisterRoutes(router)
}
