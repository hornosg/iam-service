// DEPRECATED: onboarding-service ahora auto-genera su service token usando JWT_SECRET.
// Este script solo es necesario como fallback si se quiere usar IAM_SUPER_ADMIN_TOKEN estático.
// Ver: onboarding-service/src/onboarding/infrastructure/auth/service_token_provider.go
//
// SEGURIDAD (cierre de bypass de tenant): este script ya NO emite tokens sin tenant_id ni
// de larga vida. Firma tenant_id (system tenant) + namespace y un exp corto, para que no
// pueda usarse como "llave maestra" cross-tenant. Acepta tenant/namespace por args.
//
// Uso: cd services/iam-service && go run scripts/generate-onboarding-token.go <JWT_SECRET> [SYSTEM_TENANT_ID] [NAMESPACE]
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Uso: go run scripts/generate-onboarding-token.go <JWT_SECRET> [SYSTEM_TENANT_ID] [NAMESPACE]\n")
		fmt.Fprintf(os.Stderr, "  Obtener: kubectl get secret iam-secrets -o jsonpath='{.data.JWT_SECRET}' -n default | base64 -d\n")
		os.Exit(1)
	}
	secret := os.Args[1]
	if len(secret) < 32 {
		fmt.Fprintf(os.Stderr, "JWT_SECRET debe tener al menos 32 caracteres\n")
		os.Exit(1)
	}

	systemTenantID := "123e4567-e89b-12d3-a456-426614174003"
	if len(os.Args) >= 3 && os.Args[2] != "" {
		systemTenantID = os.Args[2]
	}
	namespace := "mc"
	if len(os.Args) >= 4 && os.Args[3] != "" {
		namespace = os.Args[3]
	}

	claims := jwt.MapClaims{
		"user_id":   "onboarding-service",
		"iss":       "iam-service",
		"tenant_id": systemTenantID,
		"namespace": namespace,
		"iat":       time.Now().Unix(),
		"exp":       time.Now().Add(1 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(signed)
}
