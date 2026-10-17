package adapter

import (
	"crypto/rsa"
	"encoding/base64"
	"math/big"
	"sort"
)

// JWK es la entrada pública de una clave de verificación (ACC-E03 T7,
// ADR-003 §d). El struct sólo tiene campos de material PÚBLICO (`n`/`e`):
// no hay forma de serializar `d`/`p`/`q` porque no existen acá — la fuga de
// la privada por el JWKS queda cerrada por tipo, no por un filtro.
type JWK struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// JWKS es el documento de `GET /.well-known/jwks.json` (RFC 7517 §5).
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// PublicJWKS publica la keyring de verificación: la MISMA que usan Parse,
// el gate de revocación y `authorize` (vía PublicKeyFor). Durante una
// rotación (ADR-003 §c, T8) la keyring tiene la clave en gracia y la nueva,
// y las dos salen acá. Orden por kid para que la respuesta sea estable.
//
// Receptor nil → JWKS vacío (mismo fail-closed que PublicKeyFor, T4b).
func (s *JWTServiceAdapter) PublicJWKS() JWKS {
	doc := JWKS{Keys: []JWK{}}
	if s == nil {
		return doc
	}
	kids := make([]string, 0, len(s.publicKeys))
	for kid := range s.publicKeys {
		kids = append(kids, kid)
	}
	sort.Strings(kids)
	for _, kid := range kids {
		doc.Keys = append(doc.Keys, rsaPublicJWK(kid, s.publicKeys[kid]))
	}
	return doc
}

// rsaPublicJWK codifica `n`/`e` en base64url sin padding (RFC 7518 §6.3.1),
// la misma codificación que el thumbprint del kid (config.SigningKeyKID).
func rsaPublicJWK(kid string, pub *rsa.PublicKey) JWK {
	return JWK{
		Kty: "RSA",
		Use: "sig",
		Alg: "RS256",
		Kid: kid,
		N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}
}
