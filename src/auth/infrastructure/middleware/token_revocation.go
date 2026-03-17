package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"iam/src/auth/domain/port"
	"iam/src/auth/domain/value_object"
)

type TokenRevocationConfig struct {
	JWTSecret      string
	AuthRepo       port.AuthRepository
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

		claims := &value_object.TokenClaims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(cfg.JWTSecret), nil
		})

		if err != nil || !token.Valid {
			c.Next()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("token_claims", claims)

		if claims.JTI != uuid.Nil {
			revoked, err := cfg.AuthRepo.IsTokenRevoked(c.Request.Context(), claims.JTI)
			if err == nil && revoked {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error": "Token has been revoked",
				})
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
