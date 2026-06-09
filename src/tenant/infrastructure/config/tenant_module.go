package config

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	sharedport "github.com/mercadocercano/go-shared/domain/port"
	"iam/src/tenant/application/usecase"
	"iam/src/tenant/infrastructure/controller"
	"iam/src/tenant/infrastructure/criteria"
	"iam/src/tenant/infrastructure/persistence/repository"
)

// SetupTenantModule configura e inicializa el módulo de tenants y retorna el caso de uso para obtener features
func SetupTenantModule(apiGroup *gin.RouterGroup, db *sql.DB, metricsRecorder sharedport.MetricsRecorder) *usecase.GetTenantFeaturesUseCase {
	// Crear repositorio PostgreSQL
	tenantRepo := repository.NewPostgresTenantRepository(db)

	// Crear casos de uso
	createTenantUseCase := usecase.NewCreateTenantUseCase(tenantRepo, metricsRecorder)
	getTenantByIDUseCase := usecase.NewGetTenantByIDUseCase(tenantRepo)
	getTenantBySlugUseCase := usecase.NewGetTenantBySlugUseCase(tenantRepo)
	updateTenantUseCase := usecase.NewUpdateTenantUseCase(tenantRepo)
	deleteTenantUseCase := usecase.NewDeleteTenantUseCase(tenantRepo)
	listTenantsUseCase := usecase.NewListTenantsUseCase(tenantRepo)
	listTenantsByCriteriaUseCase := usecase.NewListTenantsByCriteriaUseCase(tenantRepo)
	setPlanUseCase := usecase.NewSetPlanUseCase(tenantRepo)
	updateTenantFeaturesUseCase := usecase.NewUpdateTenantFeaturesUseCase(tenantRepo)
	getTenantFeaturesUseCase := usecase.NewGetTenantFeaturesUseCase(tenantRepo)

	// Crear criteria builder
	tenantCriteriaBuilder := criteria.NewTenantCriteriaBuilder()

	// Configurar controlador HTTP
	tenantHandler := controller.NewTenantHandler(
		createTenantUseCase,
		getTenantByIDUseCase,
		getTenantBySlugUseCase,
		updateTenantUseCase,
		deleteTenantUseCase,
		listTenantsUseCase,
		listTenantsByCriteriaUseCase,
		setPlanUseCase,
		updateTenantFeaturesUseCase,
		tenantCriteriaBuilder,
	)

	// Registrar rutas HTTP
	tenantHandler.RegisterRoutes(apiGroup)

	return getTenantFeaturesUseCase
}
