package response

import (
	"github.com/google/uuid"
)

type LoginResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	ExpiresIn    int      `json:"expires_in"`
	TokenType    string   `json:"token_type"`
	User         UserData `json:"user"`
}

type UserData struct {
	ID       uuid.UUID `json:"id"`
	Email    string    `json:"email"`
	TenantID uuid.UUID `json:"tenant_id"`
	RoleID   uuid.UUID `json:"role_id"`
	Status   string    `json:"status"`
}

func NewLoginResponse(accessToken, refreshToken string, expiresIn int, user UserData) *LoginResponse {
	return &LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    expiresIn,
		TokenType:    "Bearer",
		User:         user,
	}
}
