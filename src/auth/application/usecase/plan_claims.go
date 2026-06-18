package usecase

import (
	"context"

	"github.com/google/uuid"

	"iam/src/auth/domain/port"
	"iam/src/auth/domain/value_object"
)

// resolvePlanClaim resuelve el plan del tenant para poblar el claim `plan` del JWT
// (ADR-003). Es FAIL-CLOSED-DEGRADADO: ante plan ausente, inactivo, o cualquier error de
// resolución devuelve el tier más restrictivo (FREE), NUNCA propaga error (un fallo al
// resolver el plan no debe impedir el login, solo aplica el límite más estricto).
//
// Siempre devuelve un PlanClaim no-nil para los tokens nuevos; los tokens viejos sin el
// claim ya se interpretan como FREE downstream.
func resolvePlanClaim(ctx context.Context, resolver port.PlanResolver, tenantID uuid.UUID) *value_object.PlanClaim {
	free := &value_object.PlanClaim{Tier: value_object.PlanTierFree}

	if resolver == nil {
		return free
	}

	resolved, err := resolver.GetPlanForTenant(ctx, tenantID)
	if err != nil || resolved == nil || !resolved.IsActive || resolved.Tier == "" {
		return free
	}

	return &value_object.PlanClaim{Tier: resolved.Tier, PlanID: resolved.PlanID}
}
