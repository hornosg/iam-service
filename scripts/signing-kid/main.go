// ACC-E03 T2 — imprime el kid determinístico (ADR-003 §c: thumbprint RFC 7638,
// "acc-<12hex>") de una clave de firma a partir de su PEM PKCS#8.
//
// Uso: go run ./scripts/signing-kid <ruta-al-PEM-privado>
//
// La DERIVACIÓN la comparte con el runtime (config.SigningKeyKID); este tool
// sólo agrega el parseo del archivo, para que el script de generación y el
// boot del servicio nunca computen kids distintos.
package main

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"

	"github.com/hornosg/iam-service/src/identity/infrastructure/config"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "Uso: go run ./scripts/signing-kid <ruta-al-PEM-privado-PKCS8>")
		os.Exit(1)
	}

	pemBytes, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "no se puede leer %s: %v\n", os.Args[1], err)
		os.Exit(1)
	}

	block, _ := pem.Decode(pemBytes)
	if block == nil {
		fmt.Fprintln(os.Stderr, "el archivo no contiene un bloque PEM")
		os.Exit(1)
	}

	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "no es una clave PKCS#8 válida: %v\n", err)
		os.Exit(1)
	}

	rsaKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		fmt.Fprintf(os.Stderr, "la clave no es RSA (got %T) — ADR-003 §a exige RS256\n", parsed)
		os.Exit(1)
	}

	fmt.Println(config.SigningKeyKID(&rsaKey.PublicKey))
}