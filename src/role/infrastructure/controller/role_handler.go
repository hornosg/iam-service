package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	httpresp "github.com/hornosg/go-shared/infrastructure/response"

	"iam/src/role/application/request"
	"iam/src/role/application/usecase"
	"iam/src/role/domain/exception"
	"iam/src/role/infrastructure/criteria"
)

type RoleHandler struct {
	createRoleUseCase          *usecase.CreateRoleUseCase
	getRoleByIDUseCase         *usecase.GetRoleByIDUseCase
	updateRoleUseCase          *usecase.UpdateRoleUseCase
	deleteRoleUseCase          *usecase.DeleteRoleUseCase
	listRolesUseCase           *usecase.ListRolesUseCase
	listRolesByCriteriaUseCase *usecase.ListRolesByCriteriaUseCase
	criteriaBuilder            *criteria.RoleCriteriaBuilder
}

func NewRoleHandler(
	createRoleUseCase *usecase.CreateRoleUseCase,
	getRoleByIDUseCase *usecase.GetRoleByIDUseCase,
	updateRoleUseCase *usecase.UpdateRoleUseCase,
	deleteRoleUseCase *usecase.DeleteRoleUseCase,
	listRolesUseCase *usecase.ListRolesUseCase,
	listRolesByCriteriaUseCase *usecase.ListRolesByCriteriaUseCase,
	criteriaBuilder *criteria.RoleCriteriaBuilder,
) *RoleHandler {
	return &RoleHandler{
		createRoleUseCase:          createRoleUseCase,
		getRoleByIDUseCase:         getRoleByIDUseCase,
		updateRoleUseCase:          updateRoleUseCase,
		deleteRoleUseCase:          deleteRoleUseCase,
		listRolesUseCase:           listRolesUseCase,
		listRolesByCriteriaUseCase: listRolesByCriteriaUseCase,
		criteriaBuilder:            criteriaBuilder,
	}
}

// POST /roles
func (h *RoleHandler) CreateRole(c *gin.Context) {
	var req request.CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.JSON(c, http.StatusBadRequest, err.Error())
		return
	}

	response, err := h.createRoleUseCase.Execute(c.Request.Context(), &req)
	if err != nil {
		switch err {
		case exception.ErrRoleAlreadyExists:
			httpresp.JSON(c, http.StatusConflict, "Role already exists")
		case exception.ErrInvalidRoleType:
			httpresp.JSON(c, http.StatusBadRequest, "Invalid role type")
		case exception.ErrInvalidTenant:
			httpresp.JSON(c, http.StatusBadRequest, "Invalid tenant")
		default:
			httpresp.JSON(c, http.StatusInternalServerError, err.Error())
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
		httpresp.JSON(c, http.StatusBadRequest, "Invalid role ID")
		return
	}

	response, err := h.getRoleByIDUseCase.Execute(c.Request.Context(), id)
	if err != nil {
		if err == exception.ErrRoleNotFound {
			httpresp.JSON(c, http.StatusNotFound, "Role not found")
			return
		}
		httpresp.JSON(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, response)
}

// PUT /roles/:id
func (h *RoleHandler) UpdateRole(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httpresp.JSON(c, http.StatusBadRequest, "Invalid role ID")
		return
	}

	var req request.UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.JSON(c, http.StatusBadRequest, err.Error())
		return
	}

	response, err := h.updateRoleUseCase.Execute(c.Request.Context(), id, &req)
	if err != nil {
		switch err {
		case exception.ErrRoleNotFound:
			httpresp.JSON(c, http.StatusNotFound, "Role not found")
		case exception.ErrSystemRoleModification:
			httpresp.JSON(c, http.StatusForbidden, "Cannot modify system role")
		default:
			httpresp.JSON(c, http.StatusInternalServerError, err.Error())
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
		httpresp.JSON(c, http.StatusBadRequest, "Invalid role ID")
		return
	}

	err = h.deleteRoleUseCase.Execute(c.Request.Context(), id)
	if err != nil {
		switch err {
		case exception.ErrRoleNotFound:
			httpresp.JSON(c, http.StatusNotFound, "Role not found")
		case exception.ErrCannotDeleteRole:
			httpresp.JSON(c, http.StatusForbidden, "Cannot delete system role")
		default:
			httpresp.JSON(c, http.StatusInternalServerError, err.Error())
		}
		return
	}

	c.JSON(http.StatusNoContent, gin.H{})
}

// GET /roles
func (h *RoleHandler) ListRoles(c *gin.Context) {
	// Construir criterios desde los query params
	criteria := h.criteriaBuilder.BuildValidated(c)

	// Ejecutar la búsqueda con criterios
	response, err := h.listRolesByCriteriaUseCase.Execute(c.Request.Context(), criteria)
	if err != nil {
		httpresp.JSON(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, response)
}

// RegisterRoutes registra las rutas del módulo role.
//
// ACC-E02 T10: `roles` es un catálogo global (sin RLS, sin tenant_id). La
// escritura (POST/PUT/DELETE) se registra en writeRouter, gated por
// `system:admin`; la lectura (GET) en readRouter, legible por autenticado con
// scope `tenant:admin` o `system:admin`. Sin este corte, un `tenant:admin`
// podía crearse un rol `SYSTEM_ADMIN` y mutar los roles de sistema existentes.
func (h *RoleHandler) RegisterRoutes(readRouter, writeRouter *gin.RouterGroup) {
	reads := readRouter.Group("/roles")
	{
		reads.GET("", h.ListRoles)
		reads.GET("/:id", h.GetRoleByID)
	}
	writes := writeRouter.Group("/roles")
	{
		writes.POST("", h.CreateRole)
		writes.PUT("/:id", h.UpdateRole)
		writes.DELETE("/:id", h.DeleteRole)
	}
}
