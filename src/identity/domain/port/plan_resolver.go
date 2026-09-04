package port

import (
	"context"

	"github.com/google/uuid"
)

// ResolvedPlan es la vista que el módulo auth necesita del plan de un tenant para firmar
// el tier en el JWT. Tipo PROPIO de auth (aislamiento de tipos, igual que ResolvedRole):
// el puerto no importa entidades del módulo plan/tenant.
type ResolvedPlan struct {
	Tier     string // FREE | BASIC | PREMIUM | ENTERPRISE
	PlanID   uuid.UUID
	IsActive bool // false si el plan está INACTIVE/DEPRECATED → caller degrada a FREE
}

// PlanResolver resuelve el plan vigente de un tenant al emitir el token (login/refresh),
// para poblar el claim `plan.tier`. Enforcement downstream offline (sin lookup por request).
type PlanResolver interface {
	// GetPlanForTenant devuelve el plan del tenant. Si el tenant no tiene plan asignado,
	// debe devolver error; el caso "plan inactivo" se expresa con IsActive=false. El
	// caller (resolvePlanClaim) decide la degradación a FREE.
	GetPlanForTenant(ctx context.Context, tenantID uuid.UUID) (*ResolvedPlan, error)
}
