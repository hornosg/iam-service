package value_object

import (
	"time"

	"github.com/google/uuid"
)

type TokenClaims struct {
	JTI       uuid.UUID       `json:"jti"`
	Issuer    string          `json:"iss"`
	Namespace string          `json:"namespace"`
	UserID    uuid.UUID       `json:"user_id"`
	Email     string          `json:"email"`
	TenantID  uuid.UUID       `json:"tenant_id"`
	RoleID    uuid.UUID       `json:"role_id"`
	// Roles son los slugs de rol que enforza go-shared.RequireRole (ej. ["cashier"]).
	// Array por extensibilidad futura (multi-rol); hoy típicamente un solo elemento.
	// Aditivo: tokens viejos sin este claim deserializan a nil.
	Roles []string `json:"roles,omitempty"`
	// Perms son los permisos finos derivados del rol (ej. ["sales:cash_session:open"]),
	// para go-shared.RequirePermission. Opcional para no inflar tokens de roles admin.
	Perms []string `json:"perms,omitempty"`
	// Plan lleva el tier del tenant para el rate limiting por plan (ADR-003). Solo el tier
	// (no la matriz de límites): cada servicio resuelve tier→límites contra una config
	// cacheada. Aditivo: tokens viejos sin este claim deserializan a nil (downstream → FREE).
	Plan      *PlanClaim      `json:"plan,omitempty"`
	Features  *TenantFeatures `json:"features"`
	ExpiresAt int64           `json:"exp"`
}

// PlanTierFree es el tier más restrictivo, usado como default fail-safe cuando el tenant
// no tiene plan, el plan está inactivo, o no se pudo resolver.
const PlanTierFree = "FREE"

// PlanClaim es la vista del plan que viaja en el JWT para el rate limiting por plan.
type PlanClaim struct {
	Tier   string    `json:"tier"`              // FREE | BASIC | PREMIUM | ENTERPRISE
	PlanID uuid.UUID `json:"plan_id,omitempty"` // trazabilidad/metering; zero si degradado a FREE
}

func NewTokenClaims(userID, tenantID, roleID uuid.UUID, email, namespace string, features *TenantFeatures, expiresAt time.Time) *TokenClaims {
	return &TokenClaims{
		JTI:       uuid.New(),
		Issuer:    "iam-service",
		Namespace: namespace,
		UserID:    userID,
		Email:     email,
		TenantID:  tenantID,
		RoleID:    roleID,
		Features:  features,
		ExpiresAt: expiresAt.Unix(),
	}
}

func (c TokenClaims) GetJTI() uuid.UUID {
	return c.JTI
}
