package usecase

import (
	"context"

	"iam/src/tenant/application/request"
	"iam/src/tenant/application/response"
	"iam/src/tenant/domain/entity"
	"iam/src/tenant/domain/exception"
	"iam/src/tenant/domain/port"
	sharedport "github.com/mercadocercano/go-shared/domain/port"
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

	// Verificar que no existe un tenant con el mismo slug
	exists, err := uc.tenantRepo.ExistsBySlug(ctx, normalizedSlug)
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
