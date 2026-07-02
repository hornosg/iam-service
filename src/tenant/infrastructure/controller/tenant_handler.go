package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	httpresp "github.com/hornosg/go-shared/infrastructure/response"

	"iam/src/tenant/application/request"
	"iam/src/tenant/application/usecase"
	"iam/src/tenant/domain/exception"
	"iam/src/tenant/infrastructure/criteria"
)

type TenantHandler struct {
	createTenantUseCase          *usecase.CreateTenantUseCase
	getTenantByIDUseCase         *usecase.GetTenantByIDUseCase
	getTenantBySlugUseCase       *usecase.GetTenantBySlugUseCase
	updateTenantUseCase          *usecase.UpdateTenantUseCase
	deleteTenantUseCase          *usecase.DeleteTenantUseCase
	listTenantsUseCase           *usecase.ListTenantsUseCase
	listTenantsByCriteriaUseCase *usecase.ListTenantsByCriteriaUseCase
	setPlanUseCase               *usecase.SetPlanUseCase
	updateTenantFeaturesUseCase  *usecase.UpdateTenantFeaturesUseCase
	criteriaBuilder              *criteria.TenantCriteriaBuilder
}

func NewTenantHandler(
	createTenantUseCase *usecase.CreateTenantUseCase,
	getTenantByIDUseCase *usecase.GetTenantByIDUseCase,
	getTenantBySlugUseCase *usecase.GetTenantBySlugUseCase,
	updateTenantUseCase *usecase.UpdateTenantUseCase,
	deleteTenantUseCase *usecase.DeleteTenantUseCase,
	listTenantsUseCase *usecase.ListTenantsUseCase,
	listTenantsByCriteriaUseCase *usecase.ListTenantsByCriteriaUseCase,
	setPlanUseCase *usecase.SetPlanUseCase,
	updateTenantFeaturesUseCase *usecase.UpdateTenantFeaturesUseCase,
	criteriaBuilder *criteria.TenantCriteriaBuilder,
) *TenantHandler {
	return &TenantHandler{
		createTenantUseCase:          createTenantUseCase,
		getTenantByIDUseCase:         getTenantByIDUseCase,
		getTenantBySlugUseCase:       getTenantBySlugUseCase,
		updateTenantUseCase:          updateTenantUseCase,
		deleteTenantUseCase:          deleteTenantUseCase,
		listTenantsUseCase:           listTenantsUseCase,
		listTenantsByCriteriaUseCase: listTenantsByCriteriaUseCase,
		setPlanUseCase:               setPlanUseCase,
		updateTenantFeaturesUseCase:  updateTenantFeaturesUseCase,
		criteriaBuilder:              criteriaBuilder,
	}
}

// RegisterProvisionRoutes expone SOLO POST /tenants bajo un grupo con scope
// tenant:provision (whatsapp-agent/onboarding). El resto de rutas de gestión quedan en
// RegisterRoutes con scope system:admin.
func (h *TenantHandler) RegisterProvisionRoutes(router *gin.RouterGroup) {
	router.POST("/tenants", h.CreateTenant)
}

// RegisterScopedRoutes expone GET/PUT/DELETE /tenants/:id bajo el grupo tenant-scoped
// (tenant:admin o system:admin). Permite a servicios S2S operar sobre su propio tenant
// sin requerir system:admin.
func (h *TenantHandler) RegisterScopedRoutes(router *gin.RouterGroup) {
	scoped := router.Group("/tenants")
	{
		scoped.GET("/:id", h.GetTenantByID)
		scoped.PUT("/:id", h.UpdateTenant)
		scoped.DELETE("/:id", h.DeleteTenant)
	}
}

// POST /tenants
func (h *TenantHandler) CreateTenant(c *gin.Context) {
	var req request.CreateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.JSON(c, http.StatusBadRequest, err.Error())
		return
	}

	response, err := h.createTenantUseCase.Execute(c.Request.Context(), &req)
	if err != nil {
		switch err {
		case exception.ErrSlugAlreadyExists:
			httpresp.JSON(c, http.StatusConflict, "Slug already exists")
		case exception.ErrDomainAlreadyExists:
			httpresp.JSON(c, http.StatusConflict, "Domain already exists")
		case exception.ErrInvalidTenantType:
			httpresp.JSON(c, http.StatusBadRequest, "Invalid tenant type")
		case exception.ErrInvalidOwner:
			httpresp.JSON(c, http.StatusBadRequest, "Invalid owner")
		default:
			httpresp.JSON(c, http.StatusInternalServerError, err.Error())
		}
		return
	}

	c.JSON(http.StatusCreated, response)
}

// GET /tenants/:id
func (h *TenantHandler) GetTenantByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httpresp.JSON(c, http.StatusBadRequest, "Invalid tenant ID")
		return
	}

	response, err := h.getTenantByIDUseCase.Execute(c.Request.Context(), id)
	if err != nil {
		if err == exception.ErrTenantNotFound {
			httpresp.JSON(c, http.StatusNotFound, "Tenant not found")
			return
		}
		httpresp.JSON(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, response)
}

// GET /tenants/by-slug/:slug
func (h *TenantHandler) GetTenantBySlug(c *gin.Context) {
	slug := c.Param("slug")
	if slug == "" {
		httpresp.JSON(c, http.StatusBadRequest, "Slug is required")
		return
	}

	response, err := h.getTenantBySlugUseCase.Execute(c.Request.Context(), slug)
	if err != nil {
		if err == exception.ErrTenantNotFound {
			httpresp.JSON(c, http.StatusNotFound, "Tenant not found")
			return
		}
		httpresp.JSON(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, response)
}

// PUT /tenants/:id
func (h *TenantHandler) UpdateTenant(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httpresp.JSON(c, http.StatusBadRequest, "Invalid tenant ID")
		return
	}

	var req request.UpdateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.JSON(c, http.StatusBadRequest, err.Error())
		return
	}

	response, err := h.updateTenantUseCase.Execute(c.Request.Context(), id, &req)
	if err != nil {
		switch err {
		case exception.ErrTenantNotFound:
			httpresp.JSON(c, http.StatusNotFound, "Tenant not found")
		case exception.ErrTenantDeleted:
			httpresp.JSON(c, http.StatusForbidden, "Cannot modify deleted tenant")
		case exception.ErrDomainAlreadyExists:
			httpresp.JSON(c, http.StatusConflict, "Domain already exists")
		case exception.ErrInvalidTenantStatus:
			httpresp.JSON(c, http.StatusBadRequest, "Invalid tenant status")
		default:
			httpresp.JSON(c, http.StatusInternalServerError, err.Error())
		}
		return
	}

	c.JSON(http.StatusOK, response)
}

// DELETE /tenants/:id
func (h *TenantHandler) DeleteTenant(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httpresp.JSON(c, http.StatusBadRequest, "Invalid tenant ID")
		return
	}

	err = h.deleteTenantUseCase.Execute(c.Request.Context(), id)
	if err != nil {
		switch err {
		case exception.ErrTenantNotFound:
			httpresp.JSON(c, http.StatusNotFound, "Tenant not found")
		case exception.ErrCannotDeleteTenant:
			httpresp.JSON(c, http.StatusForbidden, "Cannot delete tenant")
		default:
			httpresp.JSON(c, http.StatusInternalServerError, err.Error())
		}
		return
	}

	c.JSON(http.StatusNoContent, gin.H{})
}

// GET /tenants
func (h *TenantHandler) ListTenants(c *gin.Context) {
	// Usar el patrón criteria para manejar todos los filtros, ordenamiento y paginación
	validCriteria := h.criteriaBuilder.BuildValidated(c)

	// Ejecutar búsqueda usando el UseCase con criteria
	result, err := h.listTenantsByCriteriaUseCase.Execute(c.Request.Context(), validCriteria)
	if err != nil {
		httpresp.JSONWithDetails(c, http.StatusInternalServerError, "Error interno del servidor", err.Error())
		return
	}

	// Transformar el resultado al formato esperado por el frontend
	// El frontend espera {tenants: [...], total_count: N, page: N, ...}
	response := gin.H{
		"tenants":     result.Items,
		"total_count": result.TotalCount,
		"page":        result.Page,
		"page_size":   result.PageSize,
		"total_pages": result.TotalPages,
		"pagination": gin.H{
			"offset":      (result.Page - 1) * result.PageSize,
			"limit":       result.PageSize,
			"total":       result.TotalCount,
			"has_next":    result.Page < result.TotalPages,
			"has_prev":    result.Page > 1,
			"total_pages": result.TotalPages,
		},
	}

	c.JSON(http.StatusOK, response)
}

// POST /tenants/:id/plan
func (h *TenantHandler) SetTenantPlan(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httpresp.JSON(c, http.StatusBadRequest, "Invalid tenant ID")
		return
	}

	var req request.SetPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.JSON(c, http.StatusBadRequest, err.Error())
		return
	}

	response, err := h.setPlanUseCase.Execute(c.Request.Context(), id, &req)
	if err != nil {
		switch err {
		case exception.ErrTenantNotFound:
			httpresp.JSON(c, http.StatusNotFound, "Tenant not found")
		case exception.ErrTenantDeleted:
			httpresp.JSON(c, http.StatusForbidden, "Cannot modify deleted tenant")
		case exception.ErrTenantNotActive:
			httpresp.JSON(c, http.StatusForbidden, "Tenant is not active")
		case exception.ErrPlanNotFound:
			httpresp.JSON(c, http.StatusBadRequest, "Plan not found")
		default:
			httpresp.JSON(c, http.StatusInternalServerError, err.Error())
		}
		return
	}

	c.JSON(http.StatusOK, response)
}

// DELETE /tenants/:id/plan
func (h *TenantHandler) RemoveTenantPlan(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httpresp.JSON(c, http.StatusBadRequest, "Invalid tenant ID")
		return
	}

	response, err := h.setPlanUseCase.RemovePlan(c.Request.Context(), id)
	if err != nil {
		switch err {
		case exception.ErrTenantNotFound:
			httpresp.JSON(c, http.StatusNotFound, "Tenant not found")
		case exception.ErrTenantDeleted:
			httpresp.JSON(c, http.StatusForbidden, "Cannot modify deleted tenant")
		default:
			httpresp.JSON(c, http.StatusInternalServerError, err.Error())
		}
		return
	}

	c.JSON(http.StatusOK, response)
}

// PATCH /tenants/:id/features
func (h *TenantHandler) UpdateTenantFeatures(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httpresp.JSON(c, http.StatusBadRequest, "Invalid tenant ID")
		return
	}

	var req struct {
		FriendsFamily    bool `json:"friends_family"`
		PremiumAnalytics bool `json:"premium_analytics"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.JSON(c, http.StatusBadRequest, err.Error())
		return
	}

	featuresRequest := &usecase.UpdateTenantFeaturesRequest{
		TenantID:         id,
		FriendsFamily:    req.FriendsFamily,
		PremiumAnalytics: req.PremiumAnalytics,
	}

	response, err := h.updateTenantFeaturesUseCase.Execute(c.Request.Context(), featuresRequest)
	if err != nil {
		switch err {
		case exception.ErrTenantNotFound:
			httpresp.JSON(c, http.StatusNotFound, "Tenant not found")
		case exception.ErrTenantDeleted:
			httpresp.JSON(c, http.StatusForbidden, "Cannot modify deleted tenant")
		default:
			httpresp.JSON(c, http.StatusInternalServerError, err.Error())
		}
		return
	}

	c.JSON(http.StatusOK, response)
}

// RegisterRoutes registra las rutas HTTP del módulo tenant (gestión global,
// requiere system:admin). Los endpoints por ID se movieron a RegisterScopedRoutes
// para permitir acceso tenant-scoped sin system:admin.
func (h *TenantHandler) RegisterRoutes(router *gin.RouterGroup) {
	tenantGroup := router.Group("/tenants")
	{
		tenantGroup.GET("", h.ListTenants)
		tenantGroup.POST("/:id/plan", h.SetTenantPlan)
		tenantGroup.DELETE("/:id/plan", h.RemoveTenantPlan)
		tenantGroup.PATCH("/:id/features", h.UpdateTenantFeatures)
	}
}

