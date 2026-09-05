package controller

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	httpresp "github.com/hornosg/go-shared/infrastructure/response"

	"github.com/hornosg/iam-service/src/identity/application/request"
	"github.com/hornosg/iam-service/src/identity/application/usecase"
	"github.com/hornosg/iam-service/src/identity/domain/value_object"
)

type AuthHandler struct {
	loginUseCase        *usecase.LoginUseCase
	refreshTokenUseCase *usecase.RefreshTokenUseCase
	logoutUseCase       *usecase.LogoutUseCase
	revokeAllUseCase    *usecase.RevokeAllUseCase
}

func NewAuthHandler(
	loginUseCase *usecase.LoginUseCase,
	refreshTokenUseCase *usecase.RefreshTokenUseCase,
	logoutUseCase *usecase.LogoutUseCase,
	revokeAllUseCase *usecase.RevokeAllUseCase,
) *AuthHandler {
	return &AuthHandler{
		loginUseCase:        loginUseCase,
		refreshTokenUseCase: refreshTokenUseCase,
		logoutUseCase:       logoutUseCase,
		revokeAllUseCase:    revokeAllUseCase,
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
		// ACC-E02 T8j, criterio (b): el detalle (nombres de campo, formato JSON)
		// va al log, no al cuerpo de la respuesta.
		log.Printf("[auth] login: body inválido: %v", err)
		httpresp.JSON(c, http.StatusBadRequest, "Datos de entrada inválidos")
		return
	}

	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")
	response, err := h.loginUseCase.ExecuteWithInfo(c.Request.Context(), &req, ipAddress, userAgent)
	if err != nil {
		switch err {
		case usecase.ErrInvalidCredentials:
			httpresp.JSON(c, http.StatusUnauthorized, "Credenciales inválidas")
		case usecase.ErrUserNotFound:
			httpresp.JSON(c, http.StatusUnauthorized, "Usuario no encontrado")
		default:
			// ACC-E02 T8j, criterio (b): ídem logout (T8g) — el detalle interno
			// (mensajes de Postgres con nombres de tabla y policy) va al log.
			log.Printf("[auth] login: error interno: %v", err)
			httpresp.JSON(c, http.StatusInternalServerError, "Error interno del servidor")
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
		httpresp.JSON(c, http.StatusBadRequest, "Refresh token requerido")
		return
	}

	// ACC-E02 T8e: el Bearer es OPCIONAL en refresh (contrato vigente con los
	// consumidores: el cuerpo sólo lleva refresh_token). Si viene —típicamente
	// ya expirado— se pasa al usecase para el cross-check de tenant. Un Bearer
	// ilegible no bloquea: la posesión del refresh token sigue siendo la
	// credencial y el usecase resuelve el tenant de la fila.
	presenterToken := ""
	if authHeader := c.GetHeader("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		presenterToken = strings.TrimPrefix(authHeader, "Bearer ")
	}

	response, err := h.refreshTokenUseCase.ExecuteWithPresenter(c.Request.Context(), req.RefreshToken, presenterToken)
	if err != nil {
		switch err {
		case usecase.ErrInvalidToken:
			httpresp.JSON(c, http.StatusUnauthorized, "Refresh token inválido")
		case usecase.ErrExpiredToken:
			httpresp.JSON(c, http.StatusUnauthorized, "Refresh token expirado")
		case usecase.ErrUserNotFound:
			httpresp.JSON(c, http.StatusUnauthorized, "Usuario no encontrado")
		default:
			// ACC-E02 T8j, criterio (b): ídem logout (T8g) — detalle al log.
			log.Printf("[auth] refresh: error interno: %v", err)
			httpresp.JSON(c, http.StatusInternalServerError, "Error interno del servidor")
		}
		return
	}

	c.JSON(http.StatusOK, response)
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
		httpresp.JSON(c, http.StatusUnauthorized, "Usuario no autenticado")
		return
	}

	userID, ok := userIDValue.(uuid.UUID)
	if !ok {
		httpresp.JSON(c, http.StatusInternalServerError, "ID de usuario inválido")
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
		// ACC-E02 T8g, criterio (C) del gate de T8f: el detalle del error
		// (sentinelas de fila de T8f, mensajes de Postgres con nombres de tabla
		// y policy) va al log correlacionable, NUNCA al cuerpo de la respuesta.
		log.Printf("[auth] logout: error interno user=%s: %v", userID, err)
		httpresp.JSON(c, http.StatusInternalServerError, "Error cerrando sesión")
		return
	}

	c.Status(http.StatusNoContent)
}

// RevokeAll revokes all active tokens for the authenticated user
func (h *AuthHandler) RevokeAll(c *gin.Context) {
	userIDValue, exists := c.Get("user_id")
	if !exists {
		httpresp.JSON(c, http.StatusUnauthorized, "Usuario no autenticado")
		return
	}

	userID, ok := userIDValue.(uuid.UUID)
	if !ok {
		httpresp.JSON(c, http.StatusInternalServerError, "ID de usuario inválido")
		return
	}

	err := h.revokeAllUseCase.Execute(c.Request.Context(), userID)
	if err != nil {
		// ACC-E02 T8g, criterio (C) del gate de T8f: ídem logout — el detalle
		// interno va al log, la respuesta no expone nada del repo.
		log.Printf("[auth] revoke-all: error interno user=%s: %v", userID, err)
		httpresp.JSON(c, http.StatusInternalServerError, "Error revocando tokens")
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
		authGroup.POST("/logout", h.Logout)
		authGroup.POST("/revoke-all", h.RevokeAll)
	}
}
