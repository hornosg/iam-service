package port

import "iam/src/identity/domain/value_object"

type JWTService interface {
	Sign(claims *value_object.TokenClaims) (string, error)
	Parse(tokenString string) (*value_object.TokenClaims, error)

	// ParseIgnoringExpiry verifica la firma HMAC y devuelve los claims sin
	// validar expiración. ACC-E02 T8e: el caso de uso de refresh lo usa sobre el
	// access token Bearer OPCIONAL del request — que típicamente YA EXPIRÓ (esa
	// es la razón del refresh) — para cruzar el tenant reclamado contra el
	// tenant resuelto del refresh token presentado. Es un dato de contexto, no
	// un gate: la firma SÍ se verifica, la expiración no.
	ParseIgnoringExpiry(tokenString string) (*value_object.TokenClaims, error)
}
