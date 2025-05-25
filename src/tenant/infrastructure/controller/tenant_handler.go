package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"iam/src/tenant/application/request"
	"iam/src/tenant/application/usecase"
	"iam/src/tenant/domain/exception"
	"iam/src/tenant/domain/value_object"
)

type TenantHandler struct {
	createTenantUseCase         *usecase.CreateTenantUseCase
	getTenantByIDUseCase        *usecase.GetTenantByIDUseCase
	getTenantBySlugUseCase      *usecase.GetTenantBySlugUseCase
	updateTenantUseCase         *usecase.UpdateTenantUseCase
	deleteTenantUseCase         *usecase.DeleteTenantUseCase
	listTenantsUseCase          *usecase.ListTenantsUseCase
	setPlanUseCase              *usecase.SetPlanUseCase
	updateTenantFeaturesUseCase *usecase.UpdateTenantFeaturesUseCase
}

func NewTenantHandler(
	createTenantUseCase *usecase.CreateTenantUseCase,
	getTenantByIDUseCase *usecase.GetTenantByIDUseCase,
	getTenantBySlugUseCase *usecase.GetTenantBySlugUseCase,
	updateTenantUseCase *usecase.UpdateTenantUseCase,
	deleteTenantUseCase *usecase.DeleteTenantUseCase,
	listTenantsUseCase *usecase.ListTenantsUseCase,
	setPlanUseCase *usecase.SetPlanUseCase,
	updateTenantFeaturesUseCase *usecase.UpdateTenantFeaturesUseCase,
) *TenantHandler {
	return &TenantHandler{
		createTenantUseCase:         createTenantUseCase,
		getTenantByIDUseCase:        getTenantByIDUseCase,
		getTenantBySlugUseCase:      getTenantBySlugUseCase,
		updateTenantUseCase:         updateTenantUseCase,
		deleteTenantUseCase:         deleteTenantUseCase,
		listTenantsUseCase:          listTenantsUseCase,
		setPlanUseCase:              setPlanUseCase,
		updateTenantFeaturesUseCase: updateTenantFeaturesUseCase,
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
	// Parámetros de paginación
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "10")

	// Filtros
	ownerIDStr := c.Query("owner_id")
	statusStr := c.Query("status")
	typeStr := c.Query("type")
	activeOnly := c.Query("active") == "true"
	expiringDaysStr := c.Query("expiring_days")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	var response interface{}

	// Filtrar por owner
	if ownerIDStr != "" {
		ownerID, parseErr := uuid.Parse(ownerIDStr)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid owner ID"})
			return
		}
		response, err = h.listTenantsUseCase.GetByOwner(c.Request.Context(), ownerID)
	} else if statusStr != "" {
		// Filtrar por status
		status, parseErr := value_object.NewTenantStatusFromString(statusStr)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant status"})
			return
		}
		response, err = h.listTenantsUseCase.GetByStatus(c.Request.Context(), status, page, pageSize)
	} else if typeStr != "" {
		// Filtrar por tipo
		tenantType, parseErr := value_object.NewTenantTypeFromString(typeStr)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant type"})
			return
		}
		response, err = h.listTenantsUseCase.GetByType(c.Request.Context(), tenantType, page, pageSize)
	} else if activeOnly {
		// Filtrar solo activos
		response, err = h.listTenantsUseCase.GetActive(c.Request.Context(), page, pageSize)
	} else if expiringDaysStr != "" {
		// Filtrar por próximos a expirar
		days, parseErr := strconv.Atoi(expiringDaysStr)
		if parseErr != nil || days < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid expiring days"})
			return
		}
		response, err = h.listTenantsUseCase.GetExpiring(c.Request.Context(), days)
	} else {
		// Lista general con paginación
		response, err = h.listTenantsUseCase.Execute(c.Request.Context(), page, pageSize)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
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
