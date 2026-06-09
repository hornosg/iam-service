package adapter

import (
	"context"

	"github.com/google/uuid"

	auth_vo "iam/src/auth/domain/value_object"
	tenantUC "iam/src/tenant/application/usecase"
)

// TenantFeaturesAdapter implements auth/domain/port.TenantService by wrapping
// GetTenantFeaturesUseCase and converting tenant_vo.TenantFeatures → auth_vo.TenantFeatures.
// This is the anti-corruption layer that keeps the auth and tenant domains decoupled.
type TenantFeaturesAdapter struct {
	inner *tenantUC.GetTenantFeaturesUseCase
}

func NewTenantFeaturesAdapter(inner *tenantUC.GetTenantFeaturesUseCase) *TenantFeaturesAdapter {
	return &TenantFeaturesAdapter{inner: inner}
}

func (a *TenantFeaturesAdapter) Execute(ctx context.Context, tenantID uuid.UUID) (*auth_vo.TenantFeatures, error) {
	f, err := a.inner.Execute(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return &auth_vo.TenantFeatures{
		FriendsFamily:    f.FriendsFamily,
		PremiumAnalytics: f.PremiumAnalytics,
	}, nil
}
