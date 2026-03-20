// Genera un JWT para que onboarding-service llame a IAM (CreateTenant, CreateUser, GetRoleByType).
// Uso: cd services/iam-service && go run scripts/generate-onboarding-token.go <JWT_SECRET>
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Uso: go run scripts/generate-onboarding-token.go <JWT_SECRET>\n")
		fmt.Fprintf(os.Stderr, "  Obtener: kubectl get secret iam-secrets -o jsonpath='{.data.JWT_SECRET}' -n default | base64 -d\n")
		os.Exit(1)
	}
	secret := os.Args[1]
	if len(secret) < 32 {
		fmt.Fprintf(os.Stderr, "JWT_SECRET debe tener al menos 32 caracteres\n")
		os.Exit(1)
	}

	claims := jwt.MapClaims{
		"user_id": "onboarding-service",
		"iat":     time.Now().Unix(),
		"exp":     time.Now().Add(365 * 24 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(signed)
}
