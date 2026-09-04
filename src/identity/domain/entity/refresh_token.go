package entity

import (
	"time"

	"github.com/google/uuid"
)

type RefreshToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	// TenantID es el tenant dueño del token (denormalizado de users, migración
	// 021 de ACC-E02 T8e). El repositorio lo resuelve al leer la fila; el caso
	// de uso de refresh lo usa para fijar app.tenant_id y correr todo el resto
	// del flujo bajo la RLS del tenant dueño.
	TenantID  uuid.UUID
	Token     string
	ExpiresAt time.Time
	CreatedAt time.Time
}

func NewRefreshToken(userID uuid.UUID, token string, expiresAt time.Time) *RefreshToken {
	return &RefreshToken{
		ID:        uuid.New(),
		UserID:    userID,
		Token:     token,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}
}

func (rt *RefreshToken) IsExpired() bool {
	return time.Now().After(rt.ExpiresAt)
}
