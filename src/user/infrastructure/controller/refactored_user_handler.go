package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"iam/src/user/application/request"
	"iam/src/user/application/usecase"
	"iam/src/user/domain/exception"
	userCriteria "iam/src/user/infrastructure/criteria"
)

// RefactoredUserHandler muestra cómo se vería el handler usando el nuevo sistema de criterios
type RefactoredUserHandler struct {
	createUserUseCase          *usecase.CreateUserUseCase
	getUserByIDUseCase         *usecase.GetUserByIDUseCase
	updateUserUseCase          *usecase.UpdateUserUseCase
	deleteUserUseCase          *usecase.DeleteUserUseCase
	listUsersByCriteriaUseCase *usecase.ListUsersByCriteriaUseCase
	criteriaBuilder            *userCriteria.UserCriteriaBuilder
}

func NewRefactoredUserHandler(
	createUserUseCase *usecase.CreateUserUseCase,
	getUserByIDUseCase *usecase.GetUserByIDUseCase,
	updateUserUseCase *usecase.UpdateUserUseCase,
	deleteUserUseCase *usecase.DeleteUserUseCase,
	listUsersByCriteriaUseCase *usecase.ListUsersByCriteriaUseCase,
) *RefactoredUserHandler {
	return &RefactoredUserHandler{
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
func (h *RefactoredUserHandler) CreateUser(c *gin.Context) {
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
		case exception.ErrWeakPassword:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Contraseña muy débil"})
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
func (h *RefactoredUserHandler) GetUserByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de usuario inválido"})
		return
	}

	user, err := h.getUserByIDUseCase.Execute(c.Request.Context(), id)
	if err != nil {
		if err == exception.ErrUserNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Usuario no encontrado"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error interno del servidor", "details": err.Error()})
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
func (h *RefactoredUserHandler) UpdateUser(c *gin.Context) {
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
// @Summary List users with advanced filtering
// @Description List users with advanced filtering, sorting and pagination using criteria pattern
// @Tags users
// @Accept json
// @Produce json
// @Param tenant_id query string false "Filter by tenant ID"
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
// @Success 200 {object} criteria.ListResponse[entity.User]
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /users [get]
func (h *RefactoredUserHandler) ListUsers(c *gin.Context) {
	// Construir criterios validados usando el builder específico
	validCriteria := h.criteriaBuilder.BuildValidated(c)

	// Ejecutar búsqueda usando el UseCase con criterios
	result, err := h.listUsersByCriteriaUseCase.Execute(c.Request.Context(), validCriteria)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
func (h *RefactoredUserHandler) DeleteUser(c *gin.Context) {
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

	c.JSON(http.StatusNoContent, gin.H{})
}

// RegisterRoutes registra las rutas HTTP del módulo user refactorizado
func (h *RefactoredUserHandler) RegisterRoutes(router *gin.RouterGroup) {
	userGroup := router.Group("/users") // Usar ruta principal
	{
		userGroup.POST("", h.CreateUser)
		userGroup.GET("", h.ListUsers)
		userGroup.GET("/:id", h.GetUserByID)
		userGroup.PUT("/:id", h.UpdateUser)
		userGroup.DELETE("/:id", h.DeleteUser)
	}
}
