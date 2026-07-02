package config

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	sharedport "github.com/hornosg/go-shared/domain/port"
	"iam/src/tenant/application/usecase"
	"iam/src/tenant/infrastructure/controller"
	"iam/src/tenant/infrastructure/criteria"
	"iam/src/tenant/infrastructure/persistence/repository"
)

// SetupTenantScopedModule expone las rutas de gestión de un tenant por ID bajo el
// grupo tenant-scoped (system:admin o tenant:admin). Esto permite que servicios
// S2S como onboarding, que operan sobre un tenant concreto, no necesiten el scope
// system:admin solo para leer/actualizar/borrar ese tenant.
func SetupTenantScopedModule(apiGroup *gin.RouterGroup, db *sql.DB, metricsRecorder sharedport.MetricsRecorder) {
	tenantRepo := repository.NewPostgresTenantRepository(db)
	getTenantByIDUseCase := usecase.NewGetTenantByIDUseCase(tenantRepo)
	updateTenantUseCase := usecase.NewUpdateTenantUseCase(tenantRepo)
	deleteTenantUseCase := usecase.NewDeleteTenantUseCase(tenantRepo)

	tenantHandler := controller.NewTenantHandler(
		nil,
		getTenantByIDUseCase,
		nil,
		updateTenantUseCase,
		deleteTenantUseCase,
		nil, nil, nil, nil,
		nil,
	)
	tenantHandler.RegisterScopedRoutes(apiGroup)
}

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

// SetupTenantProvisionModule expone SOLO POST /tenants bajo un grupo con scope
// tenant:provision. Se usa para que whatsapp-agent cree tenants sin darle
// acceso de lectura/escritura al resto de la gestión de tenants.
func SetupTenantProvisionModule(apiGroup *gin.RouterGroup, db *sql.DB, metricsRecorder sharedport.MetricsRecorder) {
	tenantRepo := repository.NewPostgresTenantRepository(db)
	createTenantUseCase := usecase.NewCreateTenantUseCase(tenantRepo, metricsRecorder)
	tenantHandler := controller.NewTenantHandler(
		createTenantUseCase,
		nil, nil, nil, nil, nil, nil, nil, nil,
		nil,
	)
	tenantHandler.RegisterProvisionRoutes(apiGroup)
}
