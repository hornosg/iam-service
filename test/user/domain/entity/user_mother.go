package entity

import (
	"time"

	"iam/src/user/domain/entity"
	"iam/src/user/domain/value_object"

	"github.com/google/uuid"
)

// UserMother implementa el patrón Object Mother para crear entities User de prueba
type UserMother struct{}

// WithDefaults crea un usuario con valores por defecto
func (UserMother) WithDefaults() *entity.User {
	email, _ := value_object.NewEmail("test@example.com")
	now := time.Now()

	user := &entity.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: "$2a$10$N9qo8uLOickgx2ZMRZoMye.IjPeOXANBjH6dqwNFgdOjFuAtaJ.L2", // "password"
		TenantID:     uuid.New(),
		RoleID:       uuid.New(),
		Status:       value_object.StatusActive,
		Provider:     "LOCAL",
		FederatedID:  "",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	return user
}

// WithID crea un usuario con un ID específico
func (u UserMother) WithID(id uuid.UUID) *entity.User {
	user := u.WithDefaults()
	user.ID = id
	return user
}

// WithEmail crea un usuario con un email específico
func (u UserMother) WithEmail(emailStr string) *entity.User {
	user := u.WithDefaults()
	email, _ := value_object.NewEmail(emailStr)
	user.Email = email
	return user
}

// WithTenant crea un usuario con un tenant específico
func (u UserMother) WithTenant(tenantID uuid.UUID) *entity.User {
	user := u.WithDefaults()
	user.TenantID = tenantID
	return user
}

// WithRole crea un usuario con un rol específico
func (u UserMother) WithRole(roleID uuid.UUID) *entity.User {
	user := u.WithDefaults()
	user.RoleID = roleID
	return user
}

// WithStatus crea un usuario con un estado específico
func (u UserMother) WithStatus(status value_object.UserStatus) *entity.User {
	user := u.WithDefaults()
	user.Status = status
	return user
}

// Pending crea un usuario con estado pendiente
func (u UserMother) Pending() *entity.User {
	user := u.WithDefaults()
	user.Status = value_object.StatusPending
	return user
}

// Inactive crea un usuario inactivo
func (u UserMother) Inactive() *entity.User {
	user := u.WithDefaults()
	user.Status = value_object.StatusInactive
	return user
}

// WithFederatedProvider crea un usuario con proveedor federado
func (u UserMother) WithFederatedProvider(provider, federatedID string) *entity.User {
	user := u.WithDefaults()
	user.Provider = provider
	user.FederatedID = federatedID
	return user
}

// Complete crea un usuario con todos los parámetros especificados
func (UserMother) Complete(id, tenantID, roleID uuid.UUID, emailStr, provider, federatedID string, status value_object.UserStatus) *entity.User {
	email, _ := value_object.NewEmail(emailStr)
	now := time.Now()

	return &entity.User{
		ID:           id,
		Email:        email,
		PasswordHash: "$2a$10$N9qo8uLOickgx2ZMRZoMye.IjPeOXANBjH6dqwNFgdOjFuAtaJ.L2",
		TenantID:     tenantID,
		RoleID:       roleID,
		Status:       status,
		Provider:     provider,
		FederatedID:  federatedID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// Create retorna una nueva instancia de UserMother
func Create() UserMother {
	return UserMother{}
}
