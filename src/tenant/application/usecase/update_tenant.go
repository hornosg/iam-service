package usecase

import (
	"context"

	"iam/src/tenant/application/request"
	"iam/src/tenant/application/response"
	"iam/src/tenant/domain/exception"
	"iam/src/tenant/domain/port"
	"iam/src/tenant/domain/value_object"

	"github.com/google/uuid"
)

type UpdateTenantUseCase struct {
	tenantRepo port.TenantRepository
}

func NewUpdateTenantUseCase(tenantRepo port.TenantRepository) *UpdateTenantUseCase {
	return &UpdateTenantUseCase{
		tenantRepo: tenantRepo,
	}
}

func (uc *UpdateTenantUseCase) Execute(ctx context.Context, id uuid.UUID, req *request.UpdateTenantRequest) (*response.TenantResponse, error) {
	// Obtener tenant existente
	tenant, err := uc.tenantRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Verificar que el tenant puede ser modificado
	if !tenant.CanBeModified() {
		return nil, exception.ErrTenantDeleted
	}

	// Actualizar detalles si se proporcionan
	if req.Name != nil && req.Description != nil {
		tenant.UpdateDetails(*req.Name, *req.Description)
	}

	// Actualizar estado si se proporciona
	if req.Status != nil {
		status, err := value_object.NewTenantStatusFromString(*req.Status)
		if err != nil {
			return nil, exception.ErrInvalidTenantStatus
		}
		tenant.ChangeStatus(status)
	}

	// Actualizar dominio personalizado si se proporciona
	if req.Domain != nil {
		normalizedDomain := req.GetNormalizedDomain()
		if normalizedDomain != nil && *normalizedDomain != tenant.Domain {
			// Verificar que el nuevo dominio no existe
			exists, err := uc.tenantRepo.ExistsByDomain(ctx, *normalizedDomain)
			if err != nil {
				return nil, err
			}
			if exists {
				return nil, exception.ErrDomainAlreadyExists
			}
			tenant.SetCustomDomain(*normalizedDomain)
		} else if normalizedDomain == nil {
			// Remover dominio personalizado
			tenant.SetCustomDomain("")
		}
	}

	// Actualizar límites de usuarios si se proporciona
	if req.MaxUsers != nil {
		tenant.UpdateUserLimits(*req.MaxUsers)
	}

	// Guardar cambios
	if err := uc.tenantRepo.Update(ctx, tenant); err != nil {
		return nil, err
	}

	return response.NewTenantResponse(tenant), nil
}
