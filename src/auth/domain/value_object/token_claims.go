package value_object

import (
	tenant_vo "iam/src/tenant/domain/value_object"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type TokenClaims struct {
	UserID    uuid.UUID                 `json:"user_id"`
	Email     string                    `json:"email"`
	TenantID  uuid.UUID                 `json:"tenant_id"`
	RoleID    uuid.UUID                 `json:"role_id"`
	Features  *tenant_vo.TenantFeatures `json:"features"`
	ExpiresAt int64                     `json:"exp"`
}

func NewTokenClaims(userID, tenantID, roleID uuid.UUID, email string, features *tenant_vo.TenantFeatures, expiresAt time.Time) *TokenClaims {
	return &TokenClaims{
		UserID:    userID,
		Email:     email,
		TenantID:  tenantID,
		RoleID:    roleID,
		Features:  features,
		ExpiresAt: expiresAt.Unix(),
	}
}

func (c TokenClaims) GetExpirationTime() (*jwt.NumericDate, error) {
	if c.ExpiresAt == 0 {
		return nil, nil
	}
	return jwt.NewNumericDate(time.Unix(c.ExpiresAt, 0)), nil
}

func (c TokenClaims) GetIssuedAt() (*jwt.NumericDate, error) {
	return nil, nil
}

func (c TokenClaims) GetNotBefore() (*jwt.NumericDate, error) {
	return nil, nil
}

func (c TokenClaims) GetIssuer() (string, error) {
	return "", nil
}

func (c TokenClaims) GetSubject() (string, error) {
	return c.Email, nil
}

func (c TokenClaims) GetAudience() (jwt.ClaimStrings, error) {
	return nil, nil
}

func (c TokenClaims) GetID() (string, error) {
	return c.UserID.String(), nil
}
