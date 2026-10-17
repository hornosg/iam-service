package usecase

import (
	"context"

	sharedport "github.com/hornosg/go-shared/domain/port"
	"github.com/hornosg/iam-service/src/tenancy/application/request"
	"github.com/hornosg/iam-service/src/tenancy/application/response"
	"github.com/hornosg/iam-service/src/tenancy/domain/entity"
	"github.com/hornosg/iam-service/src/tenancy/domain/exception"
	"github.com/hornosg/iam-service/src/tenancy/domain/port"
	"github.com/hornosg/iam-service/src/tenancy/domain/value_object"
)

type CreateTenantUseCase struct {
	tenantRepo port.TenantRepository
	metrics    sharedport.MetricsRecorder
}

func NewCreateTenantUseCase(tenantRepo port.TenantRepository, metrics sharedport.MetricsRecorder) *CreateTenantUseCase {
	return &CreateTenantUseCase{
		tenantRepo: tenantRepo,
		metrics:    metrics,
	}
}

func (uc *CreateTenantUseCase) Execute(ctx context.Context, req *request.CreateTenantRequest) (*response.TenantResponse, error) {
	namespace, err := value_object.ResolveProductNamespace(req.Namespace)
	if err != nil {
		return nil, err
	}

	// Obtener tipo de tenant
	tenantType, err := req.GetTenantType()
	if err != nil {
		return nil, exception.ErrInvalidTenantType
	}

	// Obtener owner ID
	ownerID, err := req.GetOwnerID()
	if err != nil {
		return nil, exception.ErrInvalidOwner
	}

	// Normalizar slug
	normalizedSlug := req.GetNormalizedSlug()

	// Precheck acotado por RLS; UNIQUE(namespace, slug) en Create es la autoridad
	// final (el caller provision no puede leer otros tenants). No ampliar permisos.
	exists, err := uc.tenantRepo.ExistsBySlug(ctx, namespace, normalizedSlug)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, exception.ErrSlugAlreadyExists
	}

	// Verificar dominio personalizado si se proporciona
	normalizedDomain := req.GetNormalizedDomain()
	if normalizedDomain != "" {
		domainExists, err := uc.tenantRepo.ExistsByDomain(ctx, normalizedDomain)
		if err != nil {
			return nil, err
		}
		if domainExists {
			return nil, exception.ErrDomainAlreadyExists
		}
	}

	// Crear la entidad
	tenant := entity.NewTenant(req.Name, normalizedSlug, req.Description, tenantType, ownerID)
	tenant.Namespace = namespace

	// Establecer dominio personalizado si se proporciona
	if normalizedDomain != "" {
		tenant.SetCustomDomain(normalizedDomain)
	}

	// Guardar en repositorio
	if err := uc.tenantRepo.Create(ctx, tenant); err != nil {
		uc.metrics.Record(sharedport.MetricEvent{
			Name:   port.MetricTenantCreated,
			Kind:   sharedport.MetricKindCounter,
			Labels: map[string]string{"plan_id": "unknown", "status": "failed"},
			Value:  1,
		})
		return nil, err
	}

	planID := "none"
	if tenant.HasPlan() {
		planID = tenant.PlanID.String()
	}
	uc.metrics.Record(sharedport.MetricEvent{
		Name:   port.MetricTenantCreated,
		Kind:   sharedport.MetricKindCounter,
		Labels: map[string]string{"plan_id": planID, "status": "success"},
		Value:  1,
	})

	return response.NewTenantResponse(tenant), nil
}
