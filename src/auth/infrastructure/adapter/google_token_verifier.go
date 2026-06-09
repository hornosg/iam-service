package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"iam/src/auth/domain/port"
)

type HTTPGoogleTokenVerifier struct {
	httpClient *http.Client
	clientID   string
}

func NewHTTPGoogleTokenVerifier(clientID string) *HTTPGoogleTokenVerifier {
	return &HTTPGoogleTokenVerifier{
		httpClient: &http.Client{},
		clientID:   clientID,
	}
}

func (v *HTTPGoogleTokenVerifier) Verify(ctx context.Context, idToken string) (port.GoogleClaims, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://oauth2.googleapis.com/tokeninfo?id_token="+idToken, nil)
	if err != nil {
		return port.GoogleClaims{}, fmt.Errorf("error creando request Google: %w", err)
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return port.GoogleClaims{}, fmt.Errorf("error verificando token de Google: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return port.GoogleClaims{}, errors.New("token de Google inválido")
	}

	var tokenInfo struct {
		Aud   string `json:"aud"`
		Sub   string `json:"sub"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenInfo); err != nil {
		return port.GoogleClaims{}, fmt.Errorf("error decodificando respuesta de Google: %w", err)
	}

	if tokenInfo.Aud != v.clientID {
		return port.GoogleClaims{}, errors.New("client ID de Google inválido")
	}

	return port.GoogleClaims{
		Sub:   tokenInfo.Sub,
		Email: tokenInfo.Email,
	}, nil
}
