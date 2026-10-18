package servicetoken

import (
	"context"
	"log/slog"
	"strings"

	"github.com/hornosg/iam-service/src/access/domain/entity"
	"github.com/hornosg/iam-service/src/access/domain/port"
)

// SlogAuditor registra cada emisión como evento JSON (stdout → Promtail → Loki). Recibe sólo los
// claims y el kid: ni el token ni la aserción llegan acá (ADR-006 «Auditoría»).
type SlogAuditor struct {
	logger *slog.Logger
}

var _ port.ServiceTokenAuditor = (*SlogAuditor)(nil)

func NewSlogAuditor(logger *slog.Logger) *SlogAuditor {
	return &SlogAuditor{logger: logger}
}

// maxLoggedRequestField acota lo pedido sin verificar (aud/scope) antes de loguearlo.
const maxLoggedRequestField = 256

// ServiceTokenRejected registra la emisión rechazada (señal de un emisor comprometido: firma válida
// pidiendo un aud o scope fuera de su registro). aud y scope vienen del cliente: slog los escapa y
// se truncan. Nunca la aserción ni el token.
func (a *SlogAuditor) ServiceTokenRejected(ctx context.Context, r port.ServiceTokenRejection) {
	a.logger.WarnContext(ctx, "s2s_token_rejected",
		slog.String("event", "s2s_token_rejected"),
		slog.String("reason", r.Reason),
		slog.String("sub", r.Subject),
		slog.String("aud", truncate(r.Audience, maxLoggedRequestField)),
		slog.String("scope", truncate(r.Scope, maxLoggedRequestField)),
	)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func (a *SlogAuditor) ServiceTokenIssued(ctx context.Context, c entity.ServiceTokenClaims, kid string) {
	a.logger.InfoContext(ctx, "s2s_token_issued",
		slog.String("event", "s2s_token_issued"),
		slog.String("sub", c.Subject),
		slog.String("aud", c.Audience),
		slog.String("scope", strings.Join(c.Scopes, " ")),
		slog.String("tenant_scope", c.TenantScope),
		slog.String("namespace", c.Namespace),
		slog.String("jti", c.JTI),
		slog.String("kid", kid),
		slog.Time("exp", c.ExpiresAt),
	)
}
