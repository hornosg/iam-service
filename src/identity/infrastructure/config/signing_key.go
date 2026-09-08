package config

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
)

// Material de firma asimétrica de tokens de usuario (ACC-E03 T2, ADR-003 §b/§c).
//
// La clave privada RSA (≥2048) llega en PEM PKCS#8 por UNA de dos vars:
//   - JWT_PRIVATE_KEY_FILE (canónica): path a archivo montado — en k3s llega
//     montado desde un Secret de Kubernetes con el mismo nombre de var.
//   - JWT_PRIVATE_KEY (inline, sólo desarrollo local): PEM multilinea en env,
//     porque `go run` no tiene archivo montado.
//
// Ambas presentes → error (ambigüedad de fuente). Ninguna → error. La validación
// es análoga a ValidateJWTSecret: el caller hace log.Fatal en GIN_MODE=release
// y warning en desarrollo (auth_module.go:42-48).
//
// NINGÚN byte de clave privada se versiona (criterio de T2): las claves viven en
// keys/, ignorado por .gitignore y verificado por git check-ignore.

const (
	minSigningKeyBits = 2048
	kidPrefix         = "acc"
	kidHexLength      = 12
)

// SigningKey es el par material-clave + kid con el que T3 firmará.
// El kid es determinístico: thumbprint RFC 7638 del material público, así dos
// claves distintas nunca comparten kid por accidente de numeración (ADR-003 §c).
type SigningKey struct {
	PrivateKey *rsa.PrivateKey
	KID        string
}

// LoadSigningKeyFromEnv carga y valida la clave de firma según ADR-003 §b.
func LoadSigningKeyFromEnv() (*SigningKey, error) {
	keyPath := os.Getenv("JWT_PRIVATE_KEY_FILE")
	inline := os.Getenv("JWT_PRIVATE_KEY")

	switch {
	case keyPath != "" && inline != "":
		return nil, errors.New("signing key: JWT_PRIVATE_KEY_FILE and JWT_PRIVATE_KEY are both set — exactly one source is allowed")
	case keyPath == "" && inline == "":
		return nil, errors.New("signing key: set JWT_PRIVATE_KEY_FILE (mounted file, canonical) or JWT_PRIVATE_KEY (inline, dev only)")
	}

	var (
		pemBytes []byte
		perms    os.FileMode
	)
	if keyPath != "" {
		info, err := os.Stat(keyPath)
		if err != nil {
			return nil, fmt.Errorf("signing key: cannot stat %s: %w", keyPath, err)
		}
		perms = info.Mode().Perm()
		// Gate L4 de T1 (revisión de @dev-security): el archivo de la privada
		// debe ser legible SÓLO por su owner. Sin bits de grupo/otros (0600 o
		// más restrictivo — 0400 de montajes read-only de k8s también pasa).
		if perms&0o077 != 0 {
			return nil, fmt.Errorf("signing key: %s has permissions %04o — private key file must be owner-only (0600)", keyPath, perms)
		}
		pemBytes, err = os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("signing key: cannot read %s: %w", keyPath, err)
		}
	} else {
		pemBytes = []byte(inline)
	}

	privateKey, err := parsePKCS8Private(pemBytes)
	if err != nil {
		return nil, fmt.Errorf("signing key: %w", err)
	}

	key := &SigningKey{PrivateKey: privateKey, KID: SigningKeyKID(&privateKey.PublicKey)}

	// JWT_KEY_ID es opcional: si el operador lo fija, actúa como pin — el kid
	// derivado del material DEBE coincidir. Detecta que el Secret montó otra
	// clave que la que se registró al generarla (ADR-003 §b: "el kid se fija
	// en config junto al par").
	if pinned := os.Getenv("JWT_KEY_ID"); pinned != "" && pinned != key.KID {
		return nil, fmt.Errorf("signing key: JWT_KEY_ID=%q does not match the kid derived from the key material (%s) — the mounted key is not the registered one", pinned, key.KID)
	}

	return key, nil
}

// parsePKCS8Private exige PEM PKCS#8 con clave RSA del tamaño mínimo.
func parsePKCS8Private(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("no PEM block found in signing key material")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("not a valid PKCS#8 key: %w", err)
	}
	rsaKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("signing key must be RSA (got %T)", parsed)
	}
	if rsaKey.N.BitLen() < minSigningKeyBits {
		return nil, fmt.Errorf("signing key must be RSA-%d or stronger (got %d bits)", minSigningKeyBits, rsaKey.N.BitLen())
	}
	return rsaKey, nil
}

// SigningKeyKID calcula el kid determinístico del material público:
// "acc-" + primeros 12 hex del SHA-256 del thumbprint canónico RFC 7638
// (miembros requeridos en orden lexicográfico, sin whitespace).
func SigningKeyKID(pub *rsa.PublicKey) string {
	canonical := map[string]string{
		"e":   base64.RawURLEncoding.EncodeToString(bigIntBytes(pub.E)),
		"kty": "RSA",
		"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
	}
	// json.Marshal de un map ordena las claves lexicográficamente (spec de Go),
	// que es exactamente el orden que exige RFC 7638 (e < kty < n).
	canonicalJSON, err := json.Marshal(canonical)
	if err != nil {
		// inalcanzable: claves string cortas y valores base64url
		panic(fmt.Sprintf("signing key: cannot encode JWK thumbprint: %v", err))
	}
	sum := sha256.Sum256(canonicalJSON)
	return kidPrefix + "-" + hex.EncodeToString(sum[:])[:kidHexLength]
}

// bigIntBytes serializa un exponente pequeño sin ceros a la izquierda.
func bigIntBytes(v int) []byte {
	b := make([]byte, 4)
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte(v)
		v >>= 8
	}
	return b[i:]
}

// La generación del par NO vive en el runtime — la clave llega montada
// (JWT_PRIVATE_KEY_FILE) y se genera con scripts/generate-signing-keys.sh.

