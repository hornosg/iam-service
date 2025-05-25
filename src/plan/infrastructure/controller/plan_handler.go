package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"iam/src/plan/application/request"
	"iam/src/plan/application/usecase"
	"iam/src/plan/domain/exception"
)

type PlanHandler struct {
	createPlanUseCase  *usecase.CreatePlanUseCase
	getPlanByIDUseCase *usecase.GetPlanByIDUseCase
	listPlansUseCase   *usecase.ListPlansUseCase
}

func NewPlanHandler(
	createPlanUseCase *usecase.CreatePlanUseCase,
	getPlanByIDUseCase *usecase.GetPlanByIDUseCase,
	listPlansUseCase *usecase.ListPlansUseCase,
) *PlanHandler {
	return &PlanHandler{
		createPlanUseCase:  createPlanUseCase,
		getPlanByIDUseCase: getPlanByIDUseCase,
		listPlansUseCase:   listPlansUseCase,
	}
}

// POST /plans
func (h *PlanHandler) CreatePlan(c *gin.Context) {
	var req request.CreatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	response, err := h.createPlanUseCase.Execute(c.Request.Context(), &req)
	if err != nil {
		switch err {
		case exception.ErrPlanAlreadyExists:
			c.JSON(http.StatusConflict, gin.H{"error": "Plan already exists"})
		case exception.ErrInvalidPlanType:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid plan type"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusCreated, response)
}

// GET /plans/:id
func (h *PlanHandler) GetPlanByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid plan ID"})
		return
	}

	response, err := h.getPlanByIDUseCase.Execute(c.Request.Context(), id)
	if err != nil {
		if err == exception.ErrPlanNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Plan not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// GET /plans
func (h *PlanHandler) ListPlans(c *gin.Context) {
	// Parámetros de paginación
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "10")
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
	if activeOnly {
		response, err = h.listPlansUseCase.GetActive(c.Request.Context())
	} else {
		response, err = h.listPlansUseCase.Execute(c.Request.Context(), page, pageSize)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// RegisterRoutes registra las rutas del módulo plan
func (h *PlanHandler) RegisterRoutes(router *gin.RouterGroup) {
	plans := router.Group("/plans")
	{
		plans.POST("", h.CreatePlan)
		plans.GET("/:id", h.GetPlanByID)
		plans.GET("", h.ListPlans)
	}
}
