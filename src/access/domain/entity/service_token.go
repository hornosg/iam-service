package entity

import "time"

// ScopeClass dice si un scope de la política es de usuario o de sistema (ADR-006 «Alcance de
// tenant»). iam deriva tenant_scope de la clase de los scopes pedidos: nunca lo pide el emisor.
type ScopeClass string

const (
	ScopeClassUser   ScopeClass = "user"
	ScopeClassSystem ScopeClass = "system"
)

// AudiencePolicy son los scopes que un emisor puede pedir para un aud, con su clase.
type AudiencePolicy struct {
	Scopes map[string]ScopeClass
}

// IssuerPolicy es la entrada versionada de un emisor (ADR-006 «Registro»): namespace de proyecto
// y, por cada aud permitido, sus scopes. No es secreta; cambiarla es un cambio de código revisado.
type IssuerPolicy struct {
	Service   string
	Namespace string
	Audiences map[string]AudiencePolicy
}

// VerifiedAssertion es lo que queda de una client_assertion después de verificar forma, firma
// contra el registro del emisor y exp/iat. El jti todavía no se consumió.
type VerifiedAssertion struct {
	Issuer    string
	KID       string
	JTI       string
	ExpiresAt time.Time
}

// ServiceTokenClaims son los claims de un token de servicio a firmar. Sin tenant, usuario ni
// roles a propósito (ADR-006 «Claims»): los aporta el Bearer del usuario, cuando existe.
type ServiceTokenClaims struct {
	Issuer      string
	Subject     string
	Audience    string
	Scopes      []string
	TokenUse    string
	TenantScope string
	Namespace   string
	IssuedAt    time.Time
	ExpiresAt   time.Time
	JTI         string
}

// IssuedServiceToken es el resultado de la emisión. Token es el JWT firmado y nunca se audita.
type IssuedServiceToken struct {
	Token  string
	KID    string
	Claims ServiceTokenClaims
}
