package controller

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"iam/src/auth/application/request"
	"iam/src/auth/application/usecase"
	"iam/src/auth/domain/value_object"
)

type AuthHandler struct {
	loginUseCase         *usecase.LoginUseCase
	refreshTokenUseCase  *usecase.RefreshTokenUseCase
	validateTokenUseCase *usecase.ValidateTokenUseCase
	logoutUseCase        *usecase.LogoutUseCase
	revokeAllUseCase     *usecase.RevokeAllUseCase
}

func NewAuthHandler(
	loginUseCase *usecase.LoginUseCase,
	refreshTokenUseCase *usecase.RefreshTokenUseCase,
	validateTokenUseCase *usecase.ValidateTokenUseCase,
	logoutUseCase *usecase.LogoutUseCase,
	revokeAllUseCase *usecase.RevokeAllUseCase,
) *AuthHandler {
	return &AuthHandler{
		loginUseCase:         loginUseCase,
		refreshTokenUseCase:  refreshTokenUseCase,
		validateTokenUseCase: validateTokenUseCase,
		logoutUseCase:        logoutUseCase,
		revokeAllUseCase:     revokeAllUseCase,
	}
}

// Login godoc
// @Summary Authenticate user
// @Description Authenticate user with email/password or Google OAuth
// @Tags auth
// @Accept json
// @Produce json
// @Param request body request.LoginRequest true "Login request"
// @Success 200 {object} response.LoginResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req request.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Datos de entrada inválidos", "details": err.Error()})
		return
	}

	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")
	response, err := h.loginUseCase.ExecuteWithInfo(c.Request.Context(), &req, ipAddress, userAgent)
	if err != nil {
		switch err {
		case usecase.ErrInvalidCredentials:
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Credenciales inválidas"})
		case usecase.ErrUserNotFound:
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuario no encontrado"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error interno del servidor", "details": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, response)
}

// RefreshToken godoc
// @Summary Refresh access token
// @Description Generate new access token using refresh token
// @Tags auth
// @Accept json
// @Produce json
// @Param request body map[string]string true "Refresh token request"
// @Success 200 {object} response.LoginResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /auth/refresh [post]
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Refresh token requerido"})
		return
	}

	response, err := h.refreshTokenUseCase.Execute(c.Request.Context(), req.RefreshToken)
	if err != nil {
		switch err {
		case usecase.ErrInvalidToken:
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh token inválido"})
		case usecase.ErrExpiredToken:
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh token expirado"})
		case usecase.ErrUserNotFound:
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuario no encontrado"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error interno del servidor", "details": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, response)
}

// ValidateToken godoc
// @Summary Validate access token
// @Description Validate JWT access token and return claims
// @Tags auth
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /auth/validate [get]
func (h *AuthHandler) ValidateToken(c *gin.Context) {
	// Extraer token del header Authorization
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token de autorización requerido"})
		return
	}

	// Verificar formato Bearer
	tokenParts := strings.Split(authHeader, " ")
	if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Formato de token inválido"})
		return
	}

	claims, err := h.validateTokenUseCase.Execute(tokenParts[1])
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token inválido", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"valid":     true,
		"user_id":   claims.UserID,
		"email":     claims.Email,
		"tenant_id": claims.TenantID,
		"role_id":   claims.RoleID,
	})
}

// Logout godoc
// @Summary Logout user
// @Description Invalidate all refresh tokens for the user
// @Tags auth
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 204 "No Content"
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	userIDValue, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuario no autenticado"})
		return
	}

	userID, ok := userIDValue.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ID de usuario inválido"})
		return
	}

	// Extract claims from context if available (set by auth middleware)
	var claims *value_object.TokenClaims
	if claimsValue, exists := c.Get("token_claims"); exists {
		if tc, ok := claimsValue.(*value_object.TokenClaims); ok {
			claims = tc
		}
	}

	err := h.logoutUseCase.Execute(c.Request.Context(), userID, claims)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error cerrando sesión", "details": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// RevokeAll revokes all active tokens for the authenticated user
func (h *AuthHandler) RevokeAll(c *gin.Context) {
	userIDValue, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuario no autenticado"})
		return
	}

	userID, ok := userIDValue.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ID de usuario inválido"})
		return
	}

	err := h.revokeAllUseCase.Execute(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error revocando tokens", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Todos los tokens han sido revocados"})
}

// RegisterRoutes registra las rutas del módulo auth
func (h *AuthHandler) RegisterRoutes(router *gin.RouterGroup) {
	authGroup := router.Group("/auth")
	{
		authGroup.POST("/login", h.Login)
		authGroup.POST("/refresh", h.RefreshToken)
		authGroup.GET("/validate", h.ValidateToken)
		authGroup.POST("/logout", h.Logout)
		authGroup.POST("/revoke-all", h.RevokeAll)
	}
}
