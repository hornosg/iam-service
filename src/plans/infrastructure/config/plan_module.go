package config

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"iam/src/plans/application/usecase"
	"iam/src/plans/infrastructure/controller"
	"iam/src/plans/infrastructure/criteria"
	"iam/src/plans/infrastructure/persistence/repository"
)

// SetupPlanModule configura e inicializa el módulo de planes
func SetupPlanModule(apiGroup *gin.RouterGroup, db *sql.DB) {
	// Crear repositorio PostgreSQL
	planRepo := repository.NewPostgresPlanRepository(db)

	// Crear casos de uso
	createPlanUseCase := usecase.NewCreatePlanUseCase(planRepo)
	getPlanByIDUseCase := usecase.NewGetPlanByIDUseCase(planRepo)
	listPlansUseCase := usecase.NewListPlansUseCase(planRepo)
	listPlansByCriteriaUseCase := usecase.NewListPlansByCriteriaUseCase(planRepo)

	// Crear criteria builder
	criteriaBuilder := criteria.NewPlanCriteriaBuilder()

	// Configurar controlador HTTP
	planHandler := controller.NewPlanHandler(
		createPlanUseCase,
		getPlanByIDUseCase,
		listPlansUseCase,
		listPlansByCriteriaUseCase,
		criteriaBuilder,
	)

	// Registrar rutas HTTP
	planHandler.RegisterRoutes(apiGroup)
}
