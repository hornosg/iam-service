package request

import (
	"errors"
	"iam/src/auth/domain/value_object"

	"github.com/google/uuid"
)

type LoginRequest struct {
	Email       string                    `json:"email" binding:"required,email"`
	Password    string                    `json:"password,omitempty"`
	Provider    value_object.AuthProvider `json:"provider" binding:"required"`
	GoogleToken string                    `json:"google_token,omitempty"`
	TenantID    *uuid.UUID                `json:"tenant_id,omitempty" binding:"-"`
}

func (lr *LoginRequest) Validate() error {
	if !lr.Provider.IsValid() {
		return ErrInvalidProvider
	}

	if lr.Provider == value_object.LocalAuth && lr.Password == "" {
		return ErrPasswordRequired
	}

	if lr.Provider == value_object.GoogleAuth && lr.GoogleToken == "" {
		return ErrGoogleTokenRequired
	}

	return nil
}

var (
	ErrInvalidProvider     = errors.New("invalid authentication provider")
	ErrPasswordRequired    = errors.New("password is required for local authentication")
	ErrGoogleTokenRequired = errors.New("google token is required for google authentication")
)
