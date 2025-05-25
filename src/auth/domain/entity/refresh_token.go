package entity

import (
	"time"

	"github.com/google/uuid"
)

type RefreshToken struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	UserID    uuid.UUID `gorm:"type:uuid;not null"`
	Token     string    `gorm:"size:255;not null;unique"`
	ExpiresAt time.Time `gorm:"not null"`
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
