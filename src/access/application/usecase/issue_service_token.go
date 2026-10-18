package usecase

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hornosg/go-shared/infrastructure/s2stoken"
	"github.com/hornosg/iam-service/src/access/domain/entity"
	"github.com/hornosg/iam-service/src/access/domain/exception"
	"github.com/hornosg/iam-service/src/access/domain/port"
)

const (
	// GrantTypeClientCredentials y ClientAssertionTypeJWTBearer son los valores del protocolo
	// (RFC 6749 §4.4 y RFC 7523 §2.2) que ADR-006 «Emisión y renovación» fija.
	GrantTypeClientCredentials   = "client_credentials"
	ClientAssertionTypeJWTBearer = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"
)

// IssueServiceTokenUseCase emite el token de servicio de ADR-006. Orquesta: la verificación
// criptográfica de la aserción y el almacén de jti son puertos.
type IssueServiceTokenUseCase struct {
	verifier port.ClientAssertionVerifier
	replay   port.AssertionReplayStore
	policies port.IssuerPolicyRegistry
	signer   port.ServiceTokenSigner
	auditor  port.ServiceTokenAuditor
	limiter  port.IssuerRateLimiter
	now      func() time.Time
	newJTI   func() string
}

var _ port.ServiceTokenIssuer = (*IssueServiceTokenUseCase)(nil)

func NewIssueServiceTokenUseCase(
	verifier port.ClientAssertionVerifier,
	replay port.AssertionReplayStore,
	policies port.IssuerPolicyRegistry,
	signer port.ServiceTokenSigner,
	auditor port.ServiceTokenAuditor,
) *IssueServiceTokenUseCase {
	return &IssueServiceTokenUseCase{
		verifier: verifier, replay: replay, policies: policies, signer: signer, auditor: auditor,
		now: time.Now, newJTI: func() string { return uuid.NewString() },
	}
}

// WithIssuerLimiter agrega el cupo por emisor (nil → sin cupo).
func (uc *IssueServiceTokenUseCase) WithIssuerLimiter(l port.IssuerRateLimiter) *IssueServiceTokenUseCase {
	uc.limiter = l
	return uc
}

// WithClock fija el reloj (tests).
func (uc *IssueServiceTokenUseCase) WithClock(now func() time.Time) *IssueServiceTokenUseCase {
	uc.now = now
	return uc
}

// Issue valida en el orden fijo de ADR-006: protocolo → forma/firma/exp (verifier) → jti →
// registro del emisor → firma del token. Ningún jti de una aserción sin verificar se escribe.
func (uc *IssueServiceTokenUseCase) Issue(ctx context.Context, cmd port.IssueServiceTokenCommand) (issued entity.IssuedServiceToken, err error) {
	var subject string // sólo se completa con la firma ya verificada
	defer func() {
		if err != nil && uc.auditor != nil {
			uc.auditor.ServiceTokenRejected(ctx, port.ServiceTokenRejection{
				Reason: rejectionReason(err), Subject: subject, Audience: cmd.Audience, Scope: cmd.Scope,
			})
		}
	}()
	if strings.TrimSpace(cmd.ClientAssertion) == "" {
		return entity.IssuedServiceToken{}, fmt.Errorf("%w: client_assertion ausente", exception.ErrServiceTokenUnauthenticated)
	}
	if cmd.GrantType != GrantTypeClientCredentials {
		return entity.IssuedServiceToken{}, fmt.Errorf("%w: grant_type no soportado", exception.ErrServiceTokenBadRequest)
	}
	if cmd.ClientAssertionType != ClientAssertionTypeJWTBearer {
		return entity.IssuedServiceToken{}, fmt.Errorf("%w: client_assertion_type no soportado", exception.ErrServiceTokenInvalidRequest)
	}

	assertion, err := uc.verifier.Verify(cmd.ClientAssertion)
	if err != nil {
		if errors.Is(err, exception.ErrServiceTokenUnauthenticated) {
			return entity.IssuedServiceToken{}, err
		}
		return entity.IssuedServiceToken{}, fmt.Errorf("%w: %v", exception.ErrServiceTokenUnauthenticated, err)
	}

	subject = assertion.Issuer
	fresh, err := uc.replay.Remember(ctx, assertion.JTI, assertion.ExpiresAt.Add(s2stoken.Leeway))
	if err != nil {
		return entity.IssuedServiceToken{}, fmt.Errorf("%w: %v", exception.ErrServiceTokenUnavailable, err)
	}
	if !fresh {
		return entity.IssuedServiceToken{}, fmt.Errorf("%w: jti de aserción repetido", exception.ErrServiceTokenUnauthenticated)
	}
	// Cupo por emisor: recién acá (firma verificada y jti nuevo), para que repetir una aserción
	// capturada no consuma el cupo de la víctima (N-1 de la revisión L4 de T2).
	if uc.limiter != nil && !uc.limiter.Allow(assertion.Issuer) {
		return entity.IssuedServiceToken{}, fmt.Errorf("%w: %s", exception.ErrServiceTokenRateLimited, assertion.Issuer)
	}

	policy, ok := uc.policies.Policy(assertion.Issuer)
	if !ok {
		// Con clave registrada pero sin política: no hay nada que pueda pedir.
		return entity.IssuedServiceToken{}, fmt.Errorf("%w: emisor %s sin política", exception.ErrServiceTokenForbidden, assertion.Issuer)
	}
	scopes, tenantScope, err := authorizeRequest(policy, cmd.Audience, cmd.Scope)
	if err != nil {
		return entity.IssuedServiceToken{}, err
	}

	iat := uc.now().UTC().Truncate(time.Second)
	claims := entity.ServiceTokenClaims{
		Issuer:      s2stoken.Issuer,
		Subject:     policy.Service,
		Audience:    cmd.Audience,
		Scopes:      scopes,
		TokenUse:    s2stoken.TokenUse,
		TenantScope: tenantScope,
		Namespace:   policy.Namespace,
		IssuedAt:    iat,
		ExpiresAt:   iat.Add(s2stoken.TTL),
		JTI:         uc.newJTI(),
	}
	token, kid, err := uc.signer.SignServiceToken(claims)
	if err != nil {
		return entity.IssuedServiceToken{}, fmt.Errorf("s2s token: firma: %w", err)
	}
	if uc.auditor != nil {
		uc.auditor.ServiceTokenIssued(ctx, claims, kid)
	}
	return entity.IssuedServiceToken{Token: token, KID: kid, Claims: claims}, nil
}

// rejectionReason traduce el error del dominio a un código estable para el evento de seguridad.
func rejectionReason(err error) string {
	switch {
	case errors.Is(err, exception.ErrServiceTokenBadRequest):
		return "unsupported_grant_type"
	case errors.Is(err, exception.ErrServiceTokenInvalidRequest):
		return "invalid_request"
	case errors.Is(err, exception.ErrServiceTokenUnauthenticated):
		return "invalid_client"
	case errors.Is(err, exception.ErrServiceTokenForbidden):
		return "invalid_scope"
	case errors.Is(err, exception.ErrServiceTokenRateLimited):
		return "slow_down"
	case errors.Is(err, exception.ErrServiceTokenUnavailable):
		return "temporarily_unavailable"
	default:
		return "server_error"
	}
}

// authorizeRequest aplica el registro: aud permitido, cada scope en la política de ese aud y
// todos de la misma clase. tenant_scope se deriva de esa clase (ADR-006 «Claims»).
func authorizeRequest(policy entity.IssuerPolicy, audience, rawScope string) ([]string, string, error) {
	audPolicy, ok := policy.Audiences[audience]
	if audience == "" || !ok {
		return nil, "", fmt.Errorf("%w: audience %q no permitido para %s", exception.ErrServiceTokenForbidden, audience, policy.Service)
	}
	requested := uniqueSorted(strings.Fields(rawScope))
	if len(requested) == 0 {
		return nil, "", fmt.Errorf("%w: scope vacío", exception.ErrServiceTokenForbidden)
	}
	var class entity.ScopeClass
	for _, s := range requested {
		c, ok := audPolicy.Scopes[s]
		if !ok {
			return nil, "", fmt.Errorf("%w: scope %q no permitido para %s→%s", exception.ErrServiceTokenForbidden, s, policy.Service, audience)
		}
		if class == "" {
			class = c
		} else if c != class {
			return nil, "", fmt.Errorf("%w: mezcla de scopes de usuario y de sistema", exception.ErrServiceTokenForbidden)
		}
	}
	switch class {
	case entity.ScopeClassSystem:
		return requested, s2stoken.TenantScopeSystem, nil
	case entity.ScopeClassUser:
		return requested, s2stoken.TenantScopeUser, nil
	default:
		return nil, "", fmt.Errorf("%w: clase de scope desconocida %q", exception.ErrServiceTokenForbidden, class)
	}
}

func uniqueSorted(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, dup := seen[s]; !dup {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
