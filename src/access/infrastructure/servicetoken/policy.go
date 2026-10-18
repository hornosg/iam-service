package servicetoken

import (
	"fmt"
	"strings"

	"github.com/hornosg/iam-service/src/access/domain/entity"
	"github.com/hornosg/iam-service/src/access/domain/port"
)

// Política versionada de emisores (ADR-006 «Registro»), organizada por namespace de proyecto.
// Reemplaza a ServicePolicy + S2S_KEY_<SERVICE> de ADR-002 a medida que cada emisor migra: cada
// uno entra en la tarea que lo migra, con scopes concretos. T2 registra sólo a catalog-bff
// (piloto de T4); sus aud hacia pim y catalog-service los enumera T4 al migrar esas llamadas.
var issuerPolicies = map[string][]entity.IssuerPolicy{
	"mc": {
		{
			Service: "catalog-bff-service",
			Audiences: map[string]entity.AudiencePolicy{
				// Lecturas por tenant también para tráfico anónimo del storefront: sistema.
				"stock-service":   {Scopes: map[string]entity.ScopeClass{"stock:read": entity.ScopeClassSystem}},
				"account-service": {Scopes: map[string]entity.ScopeClass{"config:read": entity.ScopeClassSystem}},
			},
		},
	},
}

// forbiddenScopes no pueden aparecer en ninguna entrada (ADR-006): los scopes amplios de ADR-002
// no se heredan al token de servicio.
var forbiddenScopes = map[string]struct{}{"system:admin": {}, "tenant:admin": {}}

// StaticPolicies implementa port.IssuerPolicyRegistry sobre la tabla versionada.
type StaticPolicies struct {
	byService map[string]entity.IssuerPolicy
}

var _ port.IssuerPolicyRegistry = (*StaticPolicies)(nil)

// NewStaticPolicies valida y aplana la tabla versionada. Un error es un bug de la política: el
// servicio no debería arrancar.
func NewStaticPolicies() (*StaticPolicies, error) {
	return newStaticPolicies(issuerPolicies)
}

func newStaticPolicies(table map[string][]entity.IssuerPolicy) (*StaticPolicies, error) {
	out := &StaticPolicies{byService: map[string]entity.IssuerPolicy{}}
	for namespace, entries := range table {
		for _, p := range entries {
			p.Namespace = namespace
			if err := validatePolicy(p); err != nil {
				return nil, err
			}
			if _, dup := out.byService[p.Service]; dup {
				return nil, fmt.Errorf("política s2s: emisor %s duplicado", p.Service)
			}
			out.byService[p.Service] = p
		}
	}
	return out, nil
}

func validatePolicy(p entity.IssuerPolicy) error {
	if strings.TrimSpace(p.Service) == "" || strings.TrimSpace(p.Namespace) == "" {
		return fmt.Errorf("política s2s: emisor o namespace vacío")
	}
	if len(p.Audiences) == 0 {
		return fmt.Errorf("política s2s: %s sin audiences", p.Service)
	}
	for aud, ap := range p.Audiences {
		if strings.TrimSpace(aud) == "" || len(ap.Scopes) == 0 {
			return fmt.Errorf("política s2s: %s con aud vacío o sin scopes", p.Service)
		}
		for scope, class := range ap.Scopes {
			if _, bad := forbiddenScopes[scope]; bad {
				return fmt.Errorf("política s2s: %s→%s no puede tener %s", p.Service, aud, scope)
			}
			if strings.ContainsAny(scope, "* \t") || !strings.Contains(scope, ":") {
				return fmt.Errorf("política s2s: %s→%s scope %q no es <recurso>:<acción> concreto", p.Service, aud, scope)
			}
			if class != entity.ScopeClassUser && class != entity.ScopeClassSystem {
				return fmt.Errorf("política s2s: %s→%s scope %s con clase inválida %q", p.Service, aud, scope, class)
			}
		}
	}
	return nil
}

func (s *StaticPolicies) Policy(service string) (entity.IssuerPolicy, bool) {
	p, ok := s.byService[service]
	return p, ok
}
