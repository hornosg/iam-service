package port

import (
	"context"
	"crypto/rsa"
	"time"

	"github.com/hornosg/iam-service/src/access/domain/entity"
)

// Puertos de la emisión de tokens de servicio (ACC-E09 T2, ADR-006 «Ubicación en la hexagonal
// de iam»). Ninguno conoce identity: el firmador recibe claims de servicio, no TokenClaims.

// ServiceTokenSigner firma un token de servicio RS256 con la clave vigente de iam y devuelve el
// kid con el que lo firmó.
type ServiceTokenSigner interface {
	SignServiceToken(claims entity.ServiceTokenClaims) (token string, kid string, err error)
}

// AssertionReplayStore consume un jti de aserción. fresh=false si ya se había visto. Un error
// significa almacén caído: el caso de uso falla cerrado (503), nunca saltea el chequeo.
type AssertionReplayStore interface {
	Remember(ctx context.Context, jti string, until time.Time) (fresh bool, err error)
}

// IssuerKeyRegistry resuelve la pública de un emisor por (emisor, kid). Es un registro propio,
// separado del keyring de verificación de tokens de usuario: el kid se busca sólo dentro del
// emisor que declara iss=sub, nunca en un mapa global.
type IssuerKeyRegistry interface {
	PublicKey(issuer, kid string) (*rsa.PublicKey, bool)
}

// IssuerPolicyRegistry devuelve la política versionada de un emisor.
type IssuerPolicyRegistry interface {
	Policy(service string) (entity.IssuerPolicy, bool)
}

// ClientAssertionVerifier valida, en este orden, forma → firma contra IssuerKeyRegistry del
// emisor que declara iss=sub → exp/iat. Todo rechazo es exception.ErrServiceTokenUnauthenticated.
type ClientAssertionVerifier interface {
	Verify(rawAssertion string) (entity.VerifiedAssertion, error)
}

// ServiceTokenAuditor registra cada emisión (sub, aud, scope, tenant_scope, jti, kid). Nunca
// recibe el token ni la aserción.
type ServiceTokenAuditor interface {
	ServiceTokenIssued(ctx context.Context, claims entity.ServiceTokenClaims, kid string)
	// ServiceTokenRejected registra cada emisión rechazada (O-2 de la revisión L4 de T2): es la señal
	// de un emisor comprometido. Nunca recibe la aserción ni el token.
	ServiceTokenRejected(ctx context.Context, rejection ServiceTokenRejection)
}

// ServiceTokenRejection describe una emisión rechazada. Subject sólo está si la firma de la
// aserción ya se verificó; Audience y Scope son lo pedido, sin verificar (el auditor los escapa).
type ServiceTokenRejection struct {
	Reason   string
	Subject  string
	Audience string
	Scope    string
}

// IssuerRateLimiter es el cupo por emisor. Se descuenta DESPUÉS de verificar la firma y de
// registrar el jti: ni la basura ni la repetición de una aserción capturada consumen el cupo.
type IssuerRateLimiter interface {
	Allow(issuer string) bool
}

// IssueServiceTokenCommand es la solicitud client-credentials (RFC 7523 §2.2).
type IssueServiceTokenCommand struct {
	GrantType           string
	ClientAssertionType string
	ClientAssertion     string
	Audience            string
	Scope               string
}

// ServiceTokenIssuer es el puerto de entrada: el caso de uso de emisión.
type ServiceTokenIssuer interface {
	Issue(ctx context.Context, cmd IssueServiceTokenCommand) (entity.IssuedServiceToken, error)
}
