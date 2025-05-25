package entity

import (
	"iam/src/user/domain/value_object"
	"time"

	"iam/src/user/domain/exception"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID           uuid.UUID
	Email        *value_object.Email
	PasswordHash string
	TenantID     uuid.UUID
	RoleID       uuid.UUID
	Status       value_object.UserStatus
	Provider     string
	FederatedID  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func NewUser(email *value_object.Email, tenantID, roleID uuid.UUID) *User {
	return &User{
		ID:        uuid.New(),
		Email:     email,
		TenantID:  tenantID,
		RoleID:    roleID,
		Status:    value_object.StatusPending,
		Provider:  "LOCAL",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func (u *User) SetPassword(password string) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.PasswordHash = string(hashedPassword)
	u.UpdatedAt = time.Now()
	return nil
}

func (u *User) ValidatePassword(password string) error {
	return bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
}

func (u *User) ChangeStatus(status value_object.UserStatus) error {
	if !status.IsValid() {
		return exception.ErrInvalidStatus
	}
	u.Status = status
	u.UpdatedAt = time.Now()
	return nil
}

func (u *User) UpdateEmail(email *value_object.Email) {
	u.Email = email
	u.UpdatedAt = time.Now()
}

func (u *User) LinkFederatedID(provider, federatedID string) {
	u.Provider = provider
	u.FederatedID = federatedID
	u.UpdatedAt = time.Now()
}

func (u *User) IsActive() bool {
	return u.Status == value_object.StatusActive
}

func (u *User) IsPending() bool {
	return u.Status == value_object.StatusPending
}
