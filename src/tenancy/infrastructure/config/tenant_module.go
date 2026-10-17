package config

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	sharedport "github.com/hornosg/go-shared/domain/port"
	"github.com/hornosg/iam-service/src/tenancy/application/usecase"
	"github.com/hornosg/iam-service/src/tenancy/infrastructure/audit"
	"github.com/hornosg/iam-service/src/tenancy/infrastructure/controller"
	"github.com/hornosg/iam-service/src/tenancy/infrastructure/criteria"
	"github.com/hornosg/iam-service/src/tenancy/infrastructure/persistence/repository"
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

// TenantConfigGates son los gates Authorize que el composition root (router.go) arma
// para las rutas de config (ADR-005 DD-1). Llegan como handlers para que tenancy no
// importe access. Ninguno es opcional: una ruta sin gate quedaría abierta.
type TenantConfigGates struct {
	Read      gin.HandlerFunc // S2S config:read, sin allowedRoles
	Set       gin.HandlerFunc // humano JWT, rol tenant_admin
	Bootstrap gin.HandlerFunc // S2S tenant:provision
}

// SetupTenantConfigModule arma el módulo de config key-value (`tenant_config`, ACC-E04 T3)
// y monta sus rutas (T4) con la cadena exigida por ADR-005: en read y bootstrap, el
// wrapper de auditoría va ANTES del gate para observar también 401/403. El emitter es el
// adapter canónico de producción (NewCanonicalLogger → stdout → Promtail/Loki).
func SetupTenantConfigModule(apiGroup *gin.RouterGroup, db *sql.DB, gates TenantConfigGates, emitter audit.Emitter) *controller.TenantConfigController {
	if gates.Read == nil || gates.Set == nil || gates.Bootstrap == nil {
		panic("SetupTenantConfigModule: los tres gates de config son obligatorios (ADR-005 DD-1)")
	}
	configRepo := repository.NewPostgresTenantConfigRepository(db)
	getConfigUseCase := usecase.NewGetTenantConfigUseCase(configRepo)
	setConfigUseCase := usecase.NewSetTenantConfigUseCase(configRepo)
	bootstrapConfigUseCase := usecase.NewBootstrapTenantConfigUseCase(configRepo)

	configController := controller.NewTenantConfigController(getConfigUseCase, setConfigUseCase, bootstrapConfigUseCase)
	configController.RegisterReadRoute(apiGroup, audit.ConfigAccess(emitter, audit.OperationRead), gates.Read)
	configController.RegisterSetRoute(apiGroup, gates.Set)
	configController.RegisterBootstrapRoute(apiGroup, audit.ConfigAccess(emitter, audit.OperationBootstrap), gates.Bootstrap)
	return configController
}
