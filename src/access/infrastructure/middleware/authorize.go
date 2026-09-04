package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"iam/src/access/infrastructure/s2s"
	sharedctx "iam/src/shared/context"
)

// ScopeMiddlewareFactory crea middlewares que requieren scopes S2S específicos.
type ScopeMiddlewareFactory struct {
	jwtSecret string
	namespace string
	registry  *s2s.Registry
}

// NewScopeMiddlewareFactory construye la factory usando el registry cargado al boot.
func NewScopeMiddlewareFactory(jwtSecret, namespace string, registry *s2s.Registry) *ScopeMiddlewareFactory {
	return &ScopeMiddlewareFactory{
		jwtSecret: jwtSecret,
		namespace: namespace,
		registry:  registry,
	}
}

// RequireScope devuelve un gin.HandlerFunc que autoriza S2S (key + scope) o JWT + rol.
// allowedRoles solo aplica al flujo humano.
func (f *ScopeMiddlewareFactory) RequireScope(requiredScope s2s.Scope, allowedRoles ...string) gin.HandlerFunc {
	return f.RequireScopes([]s2s.Scope{requiredScope}, allowedRoles...)
}

// RequireScopes es como RequireScope pero permite múltiples scopes S2S. Útil para
// grupos de rutas que deben aceptar servicios con distintos privilegios (ej.
// tenant-scoped: system:admin o tenant:admin).
func (f *ScopeMiddlewareFactory) RequireScopes(requiredScopes []s2s.Scope, allowedRoles ...string) gin.HandlerFunc {
	return Authorize(f.jwtSecret, f.namespace, f.registry, requiredScopes, allowedRoles...)
}

// Authorize es el gate de acceso a los endpoints de gestión del IAM (tenants,
// plans, users, roles). Cierra el agujero histórico: estos endpoints confiaban en
// que el API gateway (Kong) autenticaba, pero el fallback anónimo de Kong dejaba
// pasar requests SIN token como `anonymous-consumer`, exponiéndolos en abierto.
//
// Autoriza a dos tipos de llamador:
//
//  1. Servicios internos (S2S): presentan X-API-Key que resuelve contra el
//     registry. Se permite SOLO si la credencial tiene el scope requerido por
//     la ruta. Nunca se saltea el scope.
//
//  2. Humanos: presentan un Bearer JWT válido (firma, namespace, expiración) cuyo
//     claim `roles` intersecta allowedRoles. Recursos globales cross-tenant
//     (tenants, plans) exigen system_admin; los tenant-scoped (users, roles) suman
//     tenant_admin.
//
// Fail-closed: cualquier otra cosa → 401 (sin/credencial inválida) o 403 (rol/scope
// insuficiente). El aislamiento por tenant de los datos devueltos es
// responsabilidad de la capa de repositorio (filtro tenant_id, RULE-04), NO de
// este gate: un token system_admin / de servicio con system:admin es cross-tenant
// por diseño.
func Authorize(jwtSecret, namespace string, registry *s2s.Registry, requiredScopes []s2s.Scope, allowedRoles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedRoles))
	for _, r := range allowedRoles {
		allowed[r] = struct{}{}
	}
	return func(c *gin.Context) {
		// 1. Llamador S2S por API key + scope. Comparación constant-time.
		provided := c.GetHeader("X-API-Key")
		if provided != "" {
			if cred, ok := registry.Lookup(provided); ok {
				if cred.HasAnyScope(requiredScopes) {
					c.Set("s2s", true)
					c.Set("s2s_service", cred.Service)
					// ACC-E02 T8d: el flag de system_admin se deriva del scope
					// `system:admin` que el registry ACABA de verificar (lookup
					// constant-time de la key) — no de ningún claim. Un caller
					// que pasó el gate con otro scope (ej. tenant:provision en
					// provisionGroup) NO lo recibe, aunque el gate lo deje pasar.
					if hasScope(cred.Scopes, s2s.ScopeSystemAdmin) {
						markSystemAdmin(c)
					}
					c.Next()
					return
				}
				// Key válida pero scope insuficiente: 403. No revelamos qué scopes tiene.
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden: insufficient scope"})
				return
			}
			// Key desconocida: no abortamos acá, cae al flujo JWT/401.
		}

		// 2. Humano por JWT + rol.
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenStr == authHeader {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Bearer token required"})
			return
		}

		claims := jwt.MapClaims{}
		_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(jwtSecret), nil
		})
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			return
		}

		if namespace != "" {
			if ns, _ := claims["namespace"].(string); ns != namespace {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Namespace mismatch: token does not belong to this project"})
				return
			}
		}

		roles := rolesClaim(claims)
		granted := false
		for _, r := range roles {
			if _, ok := allowed[r]; ok {
				granted = true
				break
			}
		}
		if !granted {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden: missing required role"})
			return
		}

		// ACC-E02 T8d: para humanos, el flag de system_admin se deriva de la
		// decisión de autorización recién tomada — el rol `system_admin` del
		// claim `roles` verificado (firma + intersección con allowedRoles) Y
		// la ruta efectivamente autorizando system_admin. Un claim como
		// `is_system_admin` que la ruta no coteja jamás se consulta: inyectarlo
		// en un token tenant_admin no abre el branch cross-tenant (test de T8d).
		if routeAllowsSystemAdmin(allowedRoles) && hasRole(roles, systemAdminRole) {
			markSystemAdmin(c)
		}

		// Contexto para handlers/middleware downstream (mismas claves que TenantValidation).
		c.Set("jwt_claims", claims)
		c.Set("roles", roles)
		if tid, ok := claims["tenant_id"].(string); ok && tid != "" {
			// ACC-E02 T8g: un token legacy sin tenant real serializa el claim en
			// cero ("00000000-..."): string no vacío, parseable a uuid.Nil. NO se
			// propaga — pisaría el tenant que el gate de revocación ya derivó del
			// user_id verificado (criterio (B) del gate de T8f) y devolvería la
			// request a un 500 fail-closed. El token de la prueba HTTP de T8g
			// reproduce exactamente ese modo de fallo.
			if parsed, perr := uuid.Parse(tid); perr == nil && parsed != uuid.Nil {
				c.Set("tenant_id", tid)
				// ACC-E02 T8b: propagar el tenant_id al context.Context de la request para
				// que los repositorios RLS-sensitivos puedan fijar app.tenant_id via
				// sharedpostgres.WithRLSInTransaction (patrón PLAT-E29 / tenant-service).
				c.Request = c.Request.WithContext(sharedctx.WithTenantID(c.Request.Context(), parsed))
			}
		}
		if uid, ok := claims["user_id"].(string); ok && uid != "" {
			if parsed, perr := uuid.Parse(uid); perr == nil {
				c.Set("user_id", parsed)
			}
		}
		c.Next()
	}
}

// systemAdminRole es el rol humano que Autorize verifica contra allowedRoles y
// del que deriva la autoridad system_admin (contraparte humana del scope S2S
// `system:admin`).
const systemAdminRole = "system_admin"

// markSystemAdmin propaga la autoridad system_admin YA VERIFICADA por este gate
// al context.Context de la request, para que los repositorios RLS-sensitivos
// fijen el GUC `app.is_system_admin` (sharedpostgres.SystemAdminVar) dentro de
// su transacción. Sólo este middleware la fija; consumirla sin haberla recibido
// es fail-closed (sharedctx.IsSystemAdminFromContext → false).
func markSystemAdmin(c *gin.Context) {
	c.Set("is_system_admin", true)
	c.Request = c.Request.WithContext(sharedctx.WithSystemAdmin(c.Request.Context()))
}

// hasScope reporta si el credential S2S verificado tiene el scope dado.
func hasScope(scopes []s2s.Scope, want s2s.Scope) bool {
	for _, s := range scopes {
		if s == want {
			return true
		}
	}
	return false
}

// hasRole reporta si el claim `roles` ya verificado contiene el rol dado.
func hasRole(roles []string, want string) bool {
	for _, r := range roles {
		if r == want {
			return true
		}
	}
	return false
}

// routeAllowsSystemAdmin reporta si la ruta que está gateando este middleware
// efectivamente autoriza el rol system_admin. El flag sólo se deriva cuando la
// ruta coteja esa autoridad: una ruta tenant-only con un token que además
// arrastre el rol system_admin no abre el branch cross-tenant.
func routeAllowsSystemAdmin(allowedRoles []string) bool {
	return hasRole(allowedRoles, systemAdminRole)
}

// rolesClaim extrae el claim `roles` tolerando que venga como []interface{} (lo
// normal al deserializar jwt.MapClaims) o como []string.
func rolesClaim(claims jwt.MapClaims) []string {
	raw, ok := claims["roles"]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
