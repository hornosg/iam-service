package entity

import (
	"time"

	"github.com/google/uuid"
)

type RefreshToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
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
