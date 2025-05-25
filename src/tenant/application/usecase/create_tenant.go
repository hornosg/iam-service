package usecase

import (
	"context"

	"iam/src/api/monitoring"
	"iam/src/tenant/application/request"
	"iam/src/tenant/application/response"
	"iam/src/tenant/domain/entity"
	"iam/src/tenant/domain/exception"
	"iam/src/tenant/domain/port"
)

type CreateTenantUseCase struct {
	tenantRepo port.TenantRepository
}

func NewCreateTenantUseCase(tenantRepo port.TenantRepository) *CreateTenantUseCase {
	return &CreateTenantUseCase{
		tenantRepo: tenantRepo,
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
		// Registrar métrica de fallo
		monitoring.RecordTenantCreated("unknown", "failed")
		return nil, err
	}

	// Registrar métrica de éxito
	planID := "none" // Valor por defecto para tenants sin plan
	if tenant.HasPlan() {
		planID = tenant.PlanID.String()
	}
	monitoring.RecordTenantCreated(planID, "success")

	return response.NewTenantResponse(tenant), nil
}
