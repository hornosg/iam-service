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
	Features  *TenantFeatures `json:"features"`
	ExpiresAt int64           `json:"exp"`
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
