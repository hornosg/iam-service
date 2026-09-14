package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	sharedservice "github.com/hornosg/go-shared/domain/service"
	httpresp "github.com/hornosg/go-shared/infrastructure/response"

	"github.com/hornosg/iam-service/src/identity/domain/port"
	"github.com/hornosg/iam-service/src/identity/domain/value_object"
	sharedctx "github.com/hornosg/iam-service/src/shared/context"
)

type TokenRevocationConfig struct {
	// JWT verifica la firma del Bearer con la MISMA instancia dual del
	// firmador (ACC-E03 T4, ADR-003 §f): RS256 por kid con la pública +
	// HS256 con el secreto viejo sólo durante la ventana de cutover. Jamás
	// `alg:none`, jamás confusión RS/HS — la selección es la del keyFunc del
	// adapter, cuya especificación viven los tests negativos de T3/T4. Un nil
	// acá NO es configurable: el wiring lo pasa siempre (fail-closed si
	// faltara — el parse de un token nil-adapter no compila, pero el guard
	// explícito abajo evita el panic y deja el request sin autenticar).
	JWT      port.JWTService
	AuthRepo port.AuthRepository
	// TenantResolver resuelve el tenant de un token legacy sin claim de
	// tenant (ACC-E02 T8g, criterio (B) del gate de T8f): a partir del
	// user_id YA verificado, sobre el pool de login. Opcional: nil desactiva
	// la derivación y mantiene el comportamiento previo (sin tenant, la
	// verificación de revocación falla cerrada con 500).
	TenantResolver port.TenantByUserResolver
	ExcludedRoutes []string
}

// TokenRevocationCheck returns a Gin middleware that validates the JWT,
// sets user_id and token_claims in context, and rejects revoked tokens (by JTI).
func TokenRevocationCheck(cfg TokenRevocationConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isRouteExcluded(c.Request.URL.Path, cfg.ExcludedRoutes) {
			c.Next()
			return
		}

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenStr == authHeader {
			c.Next()
			return
		}

		// ACC-E03 T4: el parse lo hace el verificador dual del firmador (misma
		// instancia, misma regla §f). Si cfg.JWT es nil el gate queda
		// fail-closed: no se puede verificar el token, no se autentica.
		var claims *value_object.TokenClaims
		if cfg.JWT != nil {
			parsed, err := cfg.JWT.Parse(tokenStr)
			if err != nil {
				c.Next()
				return
			}
			claims = parsed
		} else {
			c.Next()
			return
		}
		c.Set("user_id", claims.UserID)
		c.Set("token_claims", claims)

		// ACC-E02 T8e: propagar el tenant de los claims YA VERIFICADOS al
		// context.Context de la request. Sin esto, IsTokenRevoked (y todo repo
		// RLS-sensitivo downstream) no ve app.tenant_id y la policy deniega
		// todo. Mismo patrón que Authorize (T8b) para los grupos con scope.
		//
		// ACC-E02 T8g (criterio (B) del gate de T8f): un token legacy sin claim
		// de tenant no queda en 500 en bloque — el tenant se deriva del user_id
		// verificado (firma comprobada recién acá), nunca de una vía no
		// verificada. Fail-closed: si el usuario no existe → 401; si la
		// resolución falla → 500 (no se puede verificar la revocación, no se
		// autoriza).
		switch {
		case claims.TenantID != uuid.Nil:
			c.Request = c.Request.WithContext(sharedctx.WithTenantID(c.Request.Context(), claims.TenantID))
		case claims.UserID != uuid.Nil && cfg.TenantResolver != nil:
			tenantID, terr := cfg.TenantResolver.ResolveTenantByUserID(c.Request.Context(), claims.UserID)
			if terr != nil {
				if errors.Is(terr, sharedservice.ErrUserNotFound) {
					httpresp.Abort(c, http.StatusUnauthorized, "Token references an unknown user")
					return
				}
				httpresp.Abort(c, http.StatusInternalServerError, "No se pudo verificar la revocación del token")
				return
			}
			c.Request = c.Request.WithContext(sharedctx.WithTenantID(c.Request.Context(), tenantID))
		}

		if claims.JTI != uuid.Nil {
			// ACC-E02 T8i: el gate coteja el JTI del token Y la marca de
			// alcance user de revoke-all contra el iat del token (todo token
			// emitido antes del corte queda revocado). Un token legacy sin
			// iat (IssuedAt=0) queda revocado por cualquier marca user viva
			// de su usuario — fail-closed.
			revoked, err := cfg.AuthRepo.IsTokenRevoked(c.Request.Context(), claims.JTI, claims.UserID, claims.IssuedAt)
			if err != nil {
				// T8e: fallar cerrado. Antes un error acá dejaba seguir (fail-
				// open) y un JTI revocado pasaba el gate. Si no se puede
				// verificar la revocación, no se autoriza el request.
				httpresp.Abort(c, http.StatusInternalServerError, "No se pudo verificar la revocación del token")
				return
			}
			if revoked {
				httpresp.Abort(c, http.StatusUnauthorized, "Token has been revoked")
				return
			}
		}

		c.Next()
	}
}

func isRouteExcluded(path string, excluded []string) bool {
	for _, route := range excluded {
		if strings.HasSuffix(route, "*") {
			prefix := strings.TrimSuffix(route, "*")
			if strings.HasPrefix(path, prefix) {
				return true
			}
		} else if path == route {
			return true
		}
	}
	return false
}
