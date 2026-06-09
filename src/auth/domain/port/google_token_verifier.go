package port

import "context"

type GoogleClaims struct {
	Sub   string
	Email string
}

type GoogleTokenVerifier interface {
	Verify(ctx context.Context, idToken string) (GoogleClaims, error)
}
