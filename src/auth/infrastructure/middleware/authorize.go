package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Authorize es el gate de acceso a los endpoints de gestión del IAM (tenants,
// plans, users, roles). Cierra el agujero histórico: estos endpoints confiaban en
// que el API gateway (Kong) autenticaba, pero el fallback anónimo de Kong dejaba
// pasar requests SIN token como `anonymous-consumer`, exponiéndolos en abierto.
//
// Autoriza a dos tipos de llamador:
//
//  1. Servicios internos (S2S): presentan X-API-Key == s2sKey → se permiten como
//     service-internal. Es el mismo secreto compartido que onboarding/sales/pim ya
//     envían; un servicio opera cross-tenant en nombre de la plataforma.
//
//  2. Humanos: presentan un Bearer JWT válido (firma, namespace, expiración) cuyo
//     claim `roles` intersecta allowedRoles. Recursos globales cross-tenant
//     (tenants, plans) exigen system_admin; los tenant-scoped (users, roles) suman
//     tenant_admin.
//
// Fail-closed: cualquier otra cosa → 401 (sin/credencial inválida) o 403 (rol
// insuficiente). El aislamiento por tenant de los datos devueltos es
// responsabilidad de la capa de repositorio (filtro tenant_id, RULE-04), NO de
// este gate: un token system_admin / de servicio es cross-tenant por diseño.
func Authorize(jwtSecret, namespace, s2sKey string, allowedRoles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedRoles))
	for _, r := range allowedRoles {
		allowed[r] = struct{}{}
	}
	return func(c *gin.Context) {
		// 1. Llamador S2S por API key (service-internal). Comparación constant-time:
		// el secreto S2S es compartido por toda la flota y la vía saltea rol/namespace,
		// así que evitamos un oráculo de timing sobre la api-key.
		if s2sKey != "" {
			provided := c.GetHeader("X-API-Key")
			if subtle.ConstantTimeCompare([]byte(provided), []byte(s2sKey)) == 1 {
				c.Set("s2s", true)
				c.Next()
				return
			}
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

		// Contexto para handlers/middleware downstream (mismas claves que TenantValidation).
		c.Set("jwt_claims", claims)
		c.Set("roles", roles)
		if tid, ok := claims["tenant_id"].(string); ok && tid != "" {
			c.Set("tenant_id", tid)
		}
		if uid, ok := claims["user_id"].(string); ok && uid != "" {
			if parsed, perr := uuid.Parse(uid); perr == nil {
				c.Set("user_id", parsed)
			}
		}
		c.Next()
	}
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
