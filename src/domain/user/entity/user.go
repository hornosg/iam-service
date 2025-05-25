package entity

import (
	"time"

	"github.com/google/uuid"
)

// User representa un usuario en el sistema IAM
// Test comment for git hook validation
type User struct {
	ID        uuid.UUID `json:"id"`
	TenantID  uuid.UUID `json:"tenant_id"`
	RoleID    uuid.UUID `json:"role_id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
