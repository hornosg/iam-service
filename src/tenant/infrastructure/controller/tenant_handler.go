package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

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

// POST /tenants
func (h *TenantHandler) CreateTenant(c *gin.Context) {
	var req request.CreateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	response, err := h.createTenantUseCase.Execute(c.Request.Context(), &req)
	if err != nil {
		switch err {
		case exception.ErrSlugAlreadyExists:
			c.JSON(http.StatusConflict, gin.H{"error": "Slug already exists"})
		case exception.ErrDomainAlreadyExists:
			c.JSON(http.StatusConflict, gin.H{"error": "Domain already exists"})
		case exception.ErrInvalidTenantType:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant type"})
		case exception.ErrInvalidOwner:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid owner"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	response, err := h.getTenantByIDUseCase.Execute(c.Request.Context(), id)
	if err != nil {
		if err == exception.ErrTenantNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Tenant not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// GET /tenants/by-slug/:slug
func (h *TenantHandler) GetTenantBySlug(c *gin.Context) {
	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Slug is required"})
		return
	}

	response, err := h.getTenantBySlugUseCase.Execute(c.Request.Context(), slug)
	if err != nil {
		if err == exception.ErrTenantNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Tenant not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// PUT /tenants/:id
func (h *TenantHandler) UpdateTenant(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	var req request.UpdateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	response, err := h.updateTenantUseCase.Execute(c.Request.Context(), id, &req)
	if err != nil {
		switch err {
		case exception.ErrTenantNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "Tenant not found"})
		case exception.ErrTenantDeleted:
			c.JSON(http.StatusForbidden, gin.H{"error": "Cannot modify deleted tenant"})
		case exception.ErrDomainAlreadyExists:
			c.JSON(http.StatusConflict, gin.H{"error": "Domain already exists"})
		case exception.ErrInvalidTenantStatus:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant status"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	err = h.deleteTenantUseCase.Execute(c.Request.Context(), id)
	if err != nil {
		switch err {
		case exception.ErrTenantNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "Tenant not found"})
		case exception.ErrCannotDeleteTenant:
			c.JSON(http.StatusForbidden, gin.H{"error": "Cannot delete tenant"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error interno del servidor", "details": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	var req request.SetPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	response, err := h.setPlanUseCase.Execute(c.Request.Context(), id, &req)
	if err != nil {
		switch err {
		case exception.ErrTenantNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "Tenant not found"})
		case exception.ErrTenantDeleted:
			c.JSON(http.StatusForbidden, gin.H{"error": "Cannot modify deleted tenant"})
		case exception.ErrTenantNotActive:
			c.JSON(http.StatusForbidden, gin.H{"error": "Tenant is not active"})
		case exception.ErrPlanNotFound:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Plan not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	response, err := h.setPlanUseCase.RemovePlan(c.Request.Context(), id)
	if err != nil {
		switch err {
		case exception.ErrTenantNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "Tenant not found"})
		case exception.ErrTenantDeleted:
			c.JSON(http.StatusForbidden, gin.H{"error": "Cannot modify deleted tenant"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	var req struct {
		FriendsFamily    bool `json:"friends_family"`
		PremiumAnalytics bool `json:"premium_analytics"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
			c.JSON(http.StatusNotFound, gin.H{"error": "Tenant not found"})
		case exception.ErrTenantDeleted:
			c.JSON(http.StatusForbidden, gin.H{"error": "Cannot modify deleted tenant"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, response)
}

// RegisterRoutes registra las rutas HTTP del módulo tenant
func (h *TenantHandler) RegisterRoutes(router *gin.RouterGroup) {
	tenantGroup := router.Group("/tenants")
	{
		tenantGroup.POST("", h.CreateTenant)
		tenantGroup.GET("", h.ListTenants)
		tenantGroup.GET("/:id", h.GetTenantByID)
		tenantGroup.GET("/by-slug/:slug", h.GetTenantBySlug)
		tenantGroup.PUT("/:id", h.UpdateTenant)
		tenantGroup.DELETE("/:id", h.DeleteTenant)
		tenantGroup.POST("/:id/plan", h.SetTenantPlan)
		tenantGroup.DELETE("/:id/plan", h.RemoveTenantPlan)
		tenantGroup.PATCH("/:id/features", h.UpdateTenantFeatures)
	}
}
