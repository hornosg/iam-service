package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"iam/src/role/application/request"
	"iam/src/role/application/usecase"
	"iam/src/role/domain/exception"
)

type RoleHandler struct {
	createRoleUseCase  *usecase.CreateRoleUseCase
	getRoleByIDUseCase *usecase.GetRoleByIDUseCase
	updateRoleUseCase  *usecase.UpdateRoleUseCase
	deleteRoleUseCase  *usecase.DeleteRoleUseCase
	listRolesUseCase   *usecase.ListRolesUseCase
}

func NewRoleHandler(
	createRoleUseCase *usecase.CreateRoleUseCase,
	getRoleByIDUseCase *usecase.GetRoleByIDUseCase,
	updateRoleUseCase *usecase.UpdateRoleUseCase,
	deleteRoleUseCase *usecase.DeleteRoleUseCase,
	listRolesUseCase *usecase.ListRolesUseCase,
) *RoleHandler {
	return &RoleHandler{
		createRoleUseCase:  createRoleUseCase,
		getRoleByIDUseCase: getRoleByIDUseCase,
		updateRoleUseCase:  updateRoleUseCase,
		deleteRoleUseCase:  deleteRoleUseCase,
		listRolesUseCase:   listRolesUseCase,
	}
}

// POST /roles
func (h *RoleHandler) CreateRole(c *gin.Context) {
	var req request.CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	response, err := h.createRoleUseCase.Execute(c.Request.Context(), &req)
	if err != nil {
		switch err {
		case exception.ErrRoleAlreadyExists:
			c.JSON(http.StatusConflict, gin.H{"error": "Role already exists"})
		case exception.ErrInvalidRoleType:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role type"})
		case exception.ErrInvalidTenant:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusCreated, response)
}

// GET /roles/:id
func (h *RoleHandler) GetRoleByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role ID"})
		return
	}

	response, err := h.getRoleByIDUseCase.Execute(c.Request.Context(), id)
	if err != nil {
		if err == exception.ErrRoleNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Role not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// PUT /roles/:id
func (h *RoleHandler) UpdateRole(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role ID"})
		return
	}

	var req request.UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	response, err := h.updateRoleUseCase.Execute(c.Request.Context(), id, &req)
	if err != nil {
		switch err {
		case exception.ErrRoleNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "Role not found"})
		case exception.ErrSystemRoleModification:
			c.JSON(http.StatusForbidden, gin.H{"error": "Cannot modify system role"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, response)
}

// DELETE /roles/:id
func (h *RoleHandler) DeleteRole(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role ID"})
		return
	}

	err = h.deleteRoleUseCase.Execute(c.Request.Context(), id)
	if err != nil {
		switch err {
		case exception.ErrRoleNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "Role not found"})
		case exception.ErrCannotDeleteRole:
			c.JSON(http.StatusForbidden, gin.H{"error": "Cannot delete system role"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusNoContent, gin.H{})
}

// GET /roles
func (h *RoleHandler) ListRoles(c *gin.Context) {
	// Parámetros de paginación
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "10")

	// Filtros
	tenantIDStr := c.Query("tenant_id")
	systemOnly := c.Query("system") == "true"
	activeOnly := c.Query("active") == "true"

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	var response interface{}

	// Filtrar por roles de sistema
	if systemOnly {
		response, err = h.listRolesUseCase.GetSystemRoles(c.Request.Context())
	} else if tenantIDStr != "" {
		// Filtrar por tenant
		tenantID, parseErr := uuid.Parse(tenantIDStr)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
			return
		}
		response, err = h.listRolesUseCase.GetByTenant(c.Request.Context(), tenantID, page, pageSize)
	} else if activeOnly {
		// Filtrar solo activos
		var tenantID *uuid.UUID
		if tenantIDStr != "" {
			tid, parseErr := uuid.Parse(tenantIDStr)
			if parseErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
				return
			}
			tenantID = &tid
		}
		response, err = h.listRolesUseCase.GetActiveRoles(c.Request.Context(), tenantID, page, pageSize)
	} else {
		// Lista general con paginación
		response, err = h.listRolesUseCase.Execute(c.Request.Context(), page, pageSize)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// RegisterRoutes registra las rutas del módulo role
func (h *RoleHandler) RegisterRoutes(router *gin.RouterGroup) {
	roles := router.Group("/roles")
	{
		roles.POST("", h.CreateRole)
		roles.GET("/:id", h.GetRoleByID)
		roles.PUT("/:id", h.UpdateRole)
		roles.DELETE("/:id", h.DeleteRole)
		roles.GET("", h.ListRoles)
	}
}
