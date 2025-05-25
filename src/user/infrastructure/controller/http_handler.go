package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"iam/src/user/application/request"
	"iam/src/user/application/usecase"
	"iam/src/user/domain/exception"
	"iam/src/user/domain/value_object"
)

type UserHandler struct {
	createUserUseCase  *usecase.CreateUserUseCase
	updateUserUseCase  *usecase.UpdateUserUseCase
	getUserByIDUseCase *usecase.GetUserByIDUseCase
	listUsersUseCase   *usecase.ListUsersUseCase
	deleteUserUseCase  *usecase.DeleteUserUseCase
}

func NewUserHandler(
	createUserUseCase *usecase.CreateUserUseCase,
	updateUserUseCase *usecase.UpdateUserUseCase,
	getUserByIDUseCase *usecase.GetUserByIDUseCase,
	listUsersUseCase *usecase.ListUsersUseCase,
	deleteUserUseCase *usecase.DeleteUserUseCase,
) *UserHandler {
	return &UserHandler{
		createUserUseCase:  createUserUseCase,
		updateUserUseCase:  updateUserUseCase,
		getUserByIDUseCase: getUserByIDUseCase,
		listUsersUseCase:   listUsersUseCase,
		deleteUserUseCase:  deleteUserUseCase,
	}
}

// CreateUser godoc
// @Summary Create a new user
// @Description Create a new user in the system
// @Tags users
// @Accept json
// @Produce json
// @Param request body request.CreateUserRequest true "Create user request"
// @Success 201 {object} response.UserResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /users [post]
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req request.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Datos de entrada inválidos", "details": err.Error()})
		return
	}

	user, err := h.createUserUseCase.Execute(c.Request.Context(), &req)
	if err != nil {
		switch err {
		case exception.ErrUserAlreadyExists:
			c.JSON(http.StatusConflict, gin.H{"error": "El usuario ya existe"})
		case exception.ErrInvalidEmail:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Email inválido"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error interno del servidor", "details": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de usuario inválido"})
		return
	}

	user, err := h.getUserByIDUseCase.Execute(c.Request.Context(), id)
	if err != nil {
		switch err {
		case exception.ErrUserNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "Usuario no encontrado"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error interno del servidor", "details": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, user)
}

// UpdateUser godoc
// @Summary Update user
// @Description Update user information
// @Tags users
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Param request body request.UpdateUserRequest true "Update user request"
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de usuario inválido"})
		return
	}

	var req request.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Datos de entrada inválidos", "details": err.Error()})
		return
	}

	req.ID = id

	user, err := h.updateUserUseCase.Execute(c.Request.Context(), &req)
	if err != nil {
		switch err {
		case exception.ErrUserNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "Usuario no encontrado"})
		case exception.ErrUserAlreadyExists:
			c.JSON(http.StatusConflict, gin.H{"error": "El email ya está en uso"})
		case exception.ErrInvalidEmail:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Email inválido"})
		case exception.ErrInvalidStatus:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Estado inválido"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error interno del servidor", "details": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, user)
}

// ListUsers godoc
// @Summary List users
// @Description List users with filtering and pagination
// @Tags users
// @Accept json
// @Produce json
// @Param tenant_id query string false "Tenant ID"
// @Param status query string false "User status"
// @Param role_id query string false "Role ID"
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Page size" default(10)
// @Success 200 {object} response.UserListResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /users [get]
func (h *UserHandler) ListUsers(c *gin.Context) {
	params := &usecase.ListUsersParams{
		Page:     1,
		PageSize: 10,
	}

	// Parse query parameters
	if pageStr := c.Query("page"); pageStr != "" {
		if page, err := strconv.Atoi(pageStr); err == nil && page > 0 {
			params.Page = page
		}
	}

	if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		if pageSize, err := strconv.Atoi(pageSizeStr); err == nil && pageSize > 0 {
			params.PageSize = pageSize
		}
	}

	if tenantIDStr := c.Query("tenant_id"); tenantIDStr != "" {
		if tenantID, err := uuid.Parse(tenantIDStr); err == nil {
			params.TenantID = &tenantID
		}
	}

	if statusStr := c.Query("status"); statusStr != "" {
		status := value_object.UserStatus(statusStr)
		if status.IsValid() {
			params.Status = &status
		}
	}

	if roleIDStr := c.Query("role_id"); roleIDStr != "" {
		if roleID, err := uuid.Parse(roleIDStr); err == nil {
			params.RoleID = &roleID
		}
	}

	users, err := h.listUsersUseCase.Execute(c.Request.Context(), params)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, users)
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de usuario inválido"})
		return
	}

	err = h.deleteUserUseCase.Execute(c.Request.Context(), id)
	if err != nil {
		switch err {
		case exception.ErrUserNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "Usuario no encontrado"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error interno del servidor", "details": err.Error()})
		}
		return
	}

	c.Status(http.StatusNoContent)
}

// RegisterRoutes registra las rutas del módulo user
func (h *UserHandler) RegisterRoutes(router *gin.RouterGroup) {
	userGroup := router.Group("/users")
	{
		userGroup.POST("", h.CreateUser)
		userGroup.GET("/:id", h.GetUserByID)
		userGroup.PUT("/:id", h.UpdateUser)
		userGroup.DELETE("/:id", h.DeleteUser)
		userGroup.GET("", h.ListUsers)
	}
}
