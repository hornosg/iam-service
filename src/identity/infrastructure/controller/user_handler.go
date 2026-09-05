package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	httpresp "github.com/hornosg/go-shared/infrastructure/response"

	"github.com/hornosg/iam-service/src/identity/application/request"
	"github.com/hornosg/iam-service/src/identity/application/usecase"
	"github.com/hornosg/iam-service/src/identity/domain/exception"
	userCriteria "github.com/hornosg/iam-service/src/identity/infrastructure/criteria"
)

// UserHandler muestra cómo se vería el handler usando el nuevo sistema de criterios
type UserHandler struct {
	createUserUseCase          *usecase.CreateUserUseCase
	getUserByIDUseCase         *usecase.GetUserByIDUseCase
	updateUserUseCase          *usecase.UpdateUserUseCase
	deleteUserUseCase          *usecase.DeleteUserUseCase
	listUsersByCriteriaUseCase *usecase.ListUsersByCriteriaUseCase
	criteriaBuilder            *userCriteria.UserCriteriaBuilder
}

func NewUserHandler(
	createUserUseCase *usecase.CreateUserUseCase,
	getUserByIDUseCase *usecase.GetUserByIDUseCase,
	updateUserUseCase *usecase.UpdateUserUseCase,
	deleteUserUseCase *usecase.DeleteUserUseCase,
	listUsersByCriteriaUseCase *usecase.ListUsersByCriteriaUseCase,
) *UserHandler {
	return &UserHandler{
		createUserUseCase:          createUserUseCase,
		getUserByIDUseCase:         getUserByIDUseCase,
		updateUserUseCase:          updateUserUseCase,
		deleteUserUseCase:          deleteUserUseCase,
		listUsersByCriteriaUseCase: listUsersByCriteriaUseCase,
		criteriaBuilder:            userCriteria.NewUserCriteriaBuilder(),
	}
}

// CreateUser godoc
// @Summary Create a new user
// @Description Create a new user in the system
// @Tags users
// @Accept json
// @Produce json
// @Param user body request.CreateUserRequest true "User creation data"
// @Success 201 {object} response.UserResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /users [post]
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req request.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.JSONWithDetails(c, http.StatusBadRequest, "Datos de entrada inválidos", err.Error())
		return
	}

	user, err := h.createUserUseCase.Execute(c.Request.Context(), &req)
	if err != nil {
		switch err {
		case exception.ErrUserAlreadyExists:
			httpresp.JSON(c, http.StatusConflict, "El usuario ya existe")
		case exception.ErrInvalidEmail:
			httpresp.JSON(c, http.StatusBadRequest, "Email inválido")
		case exception.ErrWeakPassword:
			httpresp.JSON(c, http.StatusBadRequest, "Contraseña muy débil")
		default:
			httpresp.JSONWithDetails(c, http.StatusInternalServerError, "Error interno del servidor", err.Error())
		}
		return
	}

	c.JSON(http.StatusCreated, user)
}

// GetUserByID godoc
// @Summary Get user by ID
// @Description Get a user by their ID
// @Tags users
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {object} response.UserResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /users/{id} [get]
func (h *UserHandler) GetUserByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httpresp.JSON(c, http.StatusBadRequest, "ID de usuario inválido")
		return
	}

	user, err := h.getUserByIDUseCase.Execute(c.Request.Context(), id)
	if err != nil {
		if err == exception.ErrUserNotFound {
			httpresp.JSON(c, http.StatusNotFound, "Usuario no encontrado")
			return
		}
		httpresp.JSONWithDetails(c, http.StatusInternalServerError, "Error interno del servidor", err.Error())
		return
	}

	if tenantID := c.GetHeader("X-Tenant-ID"); tenantID != "" && user.TenantID.String() != tenantID {
		httpresp.JSON(c, http.StatusNotFound, "Usuario no encontrado")
		return
	}

	c.JSON(http.StatusOK, user)
}

// UpdateUser godoc
// @Summary Update user
// @Description Update a user by ID
// @Tags users
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Param user body request.UpdateUserRequest true "User update data"
// @Success 200 {object} response.UserResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /users/{id} [put]
func (h *UserHandler) UpdateUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httpresp.JSON(c, http.StatusBadRequest, "ID de usuario inválido")
		return
	}

	var req request.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.JSONWithDetails(c, http.StatusBadRequest, "Datos de entrada inválidos", err.Error())
		return
	}

	req.ID = id

	if tenantID := c.GetHeader("X-Tenant-ID"); tenantID != "" {
		existing, err := h.getUserByIDUseCase.Execute(c.Request.Context(), id)
		if err != nil || existing.TenantID.String() != tenantID {
			httpresp.JSON(c, http.StatusNotFound, "Usuario no encontrado")
			return
		}
	}

	user, err := h.updateUserUseCase.Execute(c.Request.Context(), &req)
	if err != nil {
		switch err {
		case exception.ErrUserNotFound:
			httpresp.JSON(c, http.StatusNotFound, "Usuario no encontrado")
		case exception.ErrUserAlreadyExists:
			httpresp.JSON(c, http.StatusConflict, "El email ya está en uso")
		case exception.ErrInvalidEmail:
			httpresp.JSON(c, http.StatusBadRequest, "Email inválido")
		case exception.ErrInvalidStatus:
			httpresp.JSON(c, http.StatusBadRequest, "Estado inválido")
		default:
			httpresp.JSONWithDetails(c, http.StatusInternalServerError, "Error interno del servidor", err.Error())
		}
		return
	}

	c.JSON(http.StatusOK, user)
}

// ListUsers godoc
// @Summary List users with advanced filtering
// @Description List users with advanced filtering, sorting and pagination using criteria pattern
// @Tags users
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param tenant_id query string false "Filter by tenant ID (optional if header provided)"
// @Param status query string false "Filter by user status (ACTIVE, INACTIVE, SUSPENDED, DELETED)"
// @Param role_id query string false "Filter by role ID"
// @Param email query string false "Filter by email (LIKE search)"
// @Param first_name query string false "Filter by first name (LIKE search)"
// @Param last_name query string false "Filter by last name (LIKE search)"
// @Param provider query string false "Filter by provider (LOCAL, GOOGLE)"
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Page size (max 100)" default(10)
// @Param sort_by query string false "Sort field" default("created_at")
// @Param sort_dir query string false "Sort direction (asc, desc)" default("desc")
// @Success 200 {object} criteria.ListResponse[response.UserResponse]
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /users [get]
func (h *UserHandler) ListUsers(c *gin.Context) {
	// Verificar que el header X-Tenant-ID esté presente
	tenantID := c.GetHeader("X-Tenant-ID")
	if tenantID == "" {
		httpresp.JSON(c, http.StatusBadRequest, "X-Tenant-ID header es requerido")
		return
	}

	// Agregar tenant_id a los query parameters para el filtrado automático
	query := c.Request.URL.Query()
	query.Set("tenant_id", tenantID)
	c.Request.URL.RawQuery = query.Encode()

	// Construir criterios validados usando el builder específico
	validCriteria := h.criteriaBuilder.BuildValidated(c)

	// Ejecutar búsqueda usando el UseCase con criterios
	result, err := h.listUsersByCriteriaUseCase.Execute(c.Request.Context(), validCriteria)
	if err != nil {
		httpresp.JSONWithDetails(c, http.StatusInternalServerError, "Error interno del servidor", err.Error())
		return
	}

	c.JSON(http.StatusOK, result)
}

// DeleteUser godoc
// @Summary Delete user
// @Description Delete a user by ID
// @Tags users
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Success 204 "No Content"
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /users/{id} [delete]
func (h *UserHandler) DeleteUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httpresp.JSON(c, http.StatusBadRequest, "ID de usuario inválido")
		return
	}

	if tenantID := c.GetHeader("X-Tenant-ID"); tenantID != "" {
		existing, err := h.getUserByIDUseCase.Execute(c.Request.Context(), id)
		if err != nil || existing.TenantID.String() != tenantID {
			httpresp.JSON(c, http.StatusNotFound, "Usuario no encontrado")
			return
		}
	}

	err = h.deleteUserUseCase.Execute(c.Request.Context(), id)
	if err != nil {
		switch err {
		case exception.ErrUserNotFound:
			httpresp.JSON(c, http.StatusNotFound, "Usuario no encontrado")
		default:
			httpresp.JSONWithDetails(c, http.StatusInternalServerError, "Error interno del servidor", err.Error())
		}
		return
	}

	c.JSON(http.StatusNoContent, gin.H{})
}

// RegisterRoutes registra las rutas HTTP del módulo user refactorizado
func (h *UserHandler) RegisterRoutes(router *gin.RouterGroup) {
	userGroup := router.Group("/users") // Usar ruta principal
	{
		userGroup.POST("", h.CreateUser)
		userGroup.GET("", h.ListUsers)
		userGroup.GET("/:id", h.GetUserByID)
		userGroup.PUT("/:id", h.UpdateUser)
		userGroup.DELETE("/:id", h.DeleteUser)
	}
}
