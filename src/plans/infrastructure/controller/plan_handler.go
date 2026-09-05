package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	httpresp "github.com/hornosg/go-shared/infrastructure/response"

	"github.com/hornosg/iam-service/src/plans/application/request"
	"github.com/hornosg/iam-service/src/plans/application/usecase"
	"github.com/hornosg/iam-service/src/plans/domain/exception"
	"github.com/hornosg/iam-service/src/plans/infrastructure/criteria"
)

type PlanHandler struct {
	createPlanUseCase          *usecase.CreatePlanUseCase
	getPlanByIDUseCase         *usecase.GetPlanByIDUseCase
	listPlansUseCase           *usecase.ListPlansUseCase
	listPlansByCriteriaUseCase *usecase.ListPlansByCriteriaUseCase
	criteriaBuilder            *criteria.PlanCriteriaBuilder
}

func NewPlanHandler(
	createPlanUseCase *usecase.CreatePlanUseCase,
	getPlanByIDUseCase *usecase.GetPlanByIDUseCase,
	listPlansUseCase *usecase.ListPlansUseCase,
	listPlansByCriteriaUseCase *usecase.ListPlansByCriteriaUseCase,
	criteriaBuilder *criteria.PlanCriteriaBuilder,
) *PlanHandler {
	return &PlanHandler{
		createPlanUseCase:          createPlanUseCase,
		getPlanByIDUseCase:         getPlanByIDUseCase,
		listPlansUseCase:           listPlansUseCase,
		listPlansByCriteriaUseCase: listPlansByCriteriaUseCase,
		criteriaBuilder:            criteriaBuilder,
	}
}

// POST /plans
func (h *PlanHandler) CreatePlan(c *gin.Context) {
	var req request.CreatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.JSON(c, http.StatusBadRequest, err.Error())
		return
	}

	response, err := h.createPlanUseCase.Execute(c.Request.Context(), &req)
	if err != nil {
		switch err {
		case exception.ErrPlanAlreadyExists:
			httpresp.JSON(c, http.StatusConflict, "Plan already exists")
		case exception.ErrInvalidPlanType:
			httpresp.JSON(c, http.StatusBadRequest, "Invalid plan type")
		default:
			httpresp.JSON(c, http.StatusInternalServerError, err.Error())
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
		httpresp.JSON(c, http.StatusBadRequest, "Invalid plan ID")
		return
	}

	response, err := h.getPlanByIDUseCase.Execute(c.Request.Context(), id)
	if err != nil {
		if err == exception.ErrPlanNotFound {
			httpresp.JSON(c, http.StatusNotFound, "Plan not found")
			return
		}
		httpresp.JSON(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, response)
}

// GET /plans
func (h *PlanHandler) ListPlans(c *gin.Context) {
	// Construir criterios desde los query params
	criteria := h.criteriaBuilder.BuildValidated(c)

	// Ejecutar la búsqueda con criterios
	response, err := h.listPlansByCriteriaUseCase.Execute(c.Request.Context(), criteria)
	if err != nil {
		httpresp.JSON(c, http.StatusInternalServerError, err.Error())
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
