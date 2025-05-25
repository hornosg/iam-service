package entity

import (
	"time"

	"iam/src/auth/domain/entity"

	"github.com/google/uuid"
)

// RefreshTokenMother implementa el patrón Object Mother para crear entities RefreshToken de prueba
type RefreshTokenMother struct{}

// WithDefaults crea un refresh token con valores por defecto
func (RefreshTokenMother) WithDefaults() *entity.RefreshToken {
	userID := uuid.New()
	token := "refresh_token_" + uuid.New().String()
	expiresAt := time.Now().Add(24 * time.Hour) // Expira en 24 horas

	return entity.NewRefreshToken(userID, token, expiresAt)
}

// WithID crea un refresh token con un ID específico
func (r RefreshTokenMother) WithID(id uuid.UUID) *entity.RefreshToken {
	token := r.WithDefaults()
	token.ID = id
	return token
}

// WithUser crea un refresh token para un usuario específico
func (r RefreshTokenMother) WithUser(userID uuid.UUID) *entity.RefreshToken {
	token := r.WithDefaults()
	token.UserID = userID
	return token
}

// WithToken crea un refresh token con un token específico
func (r RefreshTokenMother) WithToken(tokenStr string) *entity.RefreshToken {
	token := r.WithDefaults()
	token.Token = tokenStr
	return token
}

// WithExpiration crea un refresh token con una fecha de expiración específica
func (r RefreshTokenMother) WithExpiration(expiresAt time.Time) *entity.RefreshToken {
	token := r.WithDefaults()
	token.ExpiresAt = expiresAt
	return token
}

// Expired crea un refresh token que ya expiró
func (r RefreshTokenMother) Expired() *entity.RefreshToken {
	token := r.WithDefaults()
	token.ExpiresAt = time.Now().Add(-1 * time.Hour) // Expiró hace 1 hora
	return token
}

// ExpiringIn crea un refresh token que expira en la duración especificada
func (r RefreshTokenMother) ExpiringIn(duration time.Duration) *entity.RefreshToken {
	token := r.WithDefaults()
	token.ExpiresAt = time.Now().Add(duration)
	return token
}

// ShortLived crea un refresh token de corta duración (15 minutos)
func (r RefreshTokenMother) ShortLived() *entity.RefreshToken {
	return r.ExpiringIn(15 * time.Minute)
}

// LongLived crea un refresh token de larga duración (30 días)
func (r RefreshTokenMother) LongLived() *entity.RefreshToken {
	return r.ExpiringIn(30 * 24 * time.Hour)
}

// WithCreatedAt crea un refresh token con una fecha de creación específica
func (r RefreshTokenMother) WithCreatedAt(createdAt time.Time) *entity.RefreshToken {
	token := r.WithDefaults()
	token.CreatedAt = createdAt
	return token
}

// Complete crea un refresh token con todos los parámetros especificados
func (RefreshTokenMother) Complete(id, userID uuid.UUID, tokenStr string, expiresAt, createdAt time.Time) *entity.RefreshToken {
	return &entity.RefreshToken{
		ID:        id,
		UserID:    userID,
		Token:     tokenStr,
		ExpiresAt: expiresAt,
		CreatedAt: createdAt,
	}
}

// ForUser crea múltiples refresh tokens para un usuario específico
func (r RefreshTokenMother) ForUser(userID uuid.UUID, count int) []*entity.RefreshToken {
	tokens := make([]*entity.RefreshToken, count)
	for i := 0; i < count; i++ {
		tokens[i] = r.WithUser(userID)
		// Hacer cada token único
		tokens[i].Token = tokens[i].Token + "_" + string(rune(i))
	}
	return tokens
}

// Create retorna una nueva instancia de RefreshTokenMother
func Create() RefreshTokenMother {
	return RefreshTokenMother{}
}
