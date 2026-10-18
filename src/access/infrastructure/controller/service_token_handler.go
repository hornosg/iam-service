package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/hornosg/iam-service/src/access/domain/exception"
	"github.com/hornosg/iam-service/src/access/domain/port"
)

// ServiceTokenPath es la ruta de emisión de ADR-006. Se monta en la raíz del engine (como el
// JWKS), fuera de /api/v1 y de sus gates de usuario: el emisor no trae un JWT de usuario.
const ServiceTokenPath = "/api/v1/s2s/token"

// maxServiceTokenRequestBytes acota el form antes de parsear: una aserción RS256 ronda 1 KB.
const maxServiceTokenRequestBytes = 8 << 10

// PreAuthLimiter es el tope previo a verificar la aserción (por IP y global).
type PreAuthLimiter interface {
	Allow(key string) bool
}

// ServiceTokenHandler expone la emisión client-credentials (RFC 6749 §4.4 + RFC 7523 §2.2).
type ServiceTokenHandler struct {
	issuer port.ServiceTokenIssuer
	perIP  PreAuthLimiter
	global PreAuthLimiter
}

func NewServiceTokenHandler(issuer port.ServiceTokenIssuer, perIP, global PreAuthLimiter) *ServiceTokenHandler {
	return &ServiceTokenHandler{issuer: issuer, perIP: perIP, global: global}
}

// Issue aplica el rate limit pre-auth y delega en el caso de uso. Cada error del dominio tiene un
// único código y nunca se degrada a otro; el cuerpo nunca repite la aserción ni el motivo interno.
func (h *ServiceTokenHandler) Issue(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	if h.issuer == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "temporarily_unavailable"})
		return
	}
	// Pre-auth: la clave es la IP de origen del socket, nunca el iss sin verificar. RemoteIP y no
	// ClientIP: con los proxies confiables por defecto de gin, X-Forwarded-For es del cliente y
	// basta rotarlo para evadir el cupo. Los emisores llegan directo por lab-network (sin ruta de Kong:
	// detrás del gateway todo compartiría su IP), así que RemoteIP es la del emisor.
	if !h.perIP.Allow(c.RemoteIP()) || !h.global.Allow("global") {
		c.Header("Retry-After", "60")
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "slow_down"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxServiceTokenRequestBytes)
	if err := c.Request.ParseForm(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}
	form := c.Request.PostForm
	issued, err := h.issuer.Issue(c.Request.Context(), port.IssueServiceTokenCommand{
		GrantType:           form.Get("grant_type"),
		ClientAssertionType: form.Get("client_assertion_type"),
		ClientAssertion:     form.Get("client_assertion"),
		Audience:            form.Get("audience"),
		Scope:               form.Get("scope"),
	})
	if err != nil {
		status, code := serviceTokenErrorStatus(err)
		if status == http.StatusTooManyRequests {
			c.Header("Retry-After", "60")
		}
		c.JSON(status, gin.H{"error": code})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"access_token": issued.Token,
		// El token viaja en X-Service-Token, nunca en Authorization (ADR-006): no es un Bearer.
		"token_type": "service",
		"expires_in": int(issued.Claims.ExpiresAt.Sub(issued.Claims.IssuedAt).Seconds()),
		"scope":      joinScopes(issued.Claims.Scopes),
	})
}

func serviceTokenErrorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, exception.ErrServiceTokenInvalidRequest):
		return http.StatusBadRequest, "invalid_request"
	case errors.Is(err, exception.ErrServiceTokenBadRequest):
		return http.StatusBadRequest, "unsupported_grant_type"
	case errors.Is(err, exception.ErrServiceTokenUnauthenticated):
		return http.StatusUnauthorized, "invalid_client"
	case errors.Is(err, exception.ErrServiceTokenForbidden):
		return http.StatusForbidden, "invalid_scope"
	case errors.Is(err, exception.ErrServiceTokenRateLimited):
		return http.StatusTooManyRequests, "slow_down"
	case errors.Is(err, exception.ErrServiceTokenUnavailable):
		return http.StatusServiceUnavailable, "temporarily_unavailable"
	default:
		return http.StatusInternalServerError, "server_error"
	}
}

func joinScopes(scopes []string) string {
	out := ""
	for i, s := range scopes {
		if i > 0 {
			out += " "
		}
		out += s
	}
	return out
}
