package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/hornosg/iam-service/src/identity/infrastructure/adapter"
)

// JWKSPath es la ruta pública del JWKS (ADR-003 §d, convención OIDC).
const JWKSPath = "/.well-known/jwks.json"

// JWKSProvider entrega la keyring pública de verificación. La implementa
// adapter.JWTServiceAdapter — el mismo verificador que firma y valida.
type JWKSProvider interface {
	PublicJWKS() adapter.JWKS
}

// JWKSHandler sirve el JWKS a los consumidores que verifican fuera de Kong
// (ACC-E03 T7: servicios Go, whatsapp-agent, web-display). Sin auth: es
// material público por definición. Responde el documento RFC 7517 tal cual,
// sin el envelope de go-shared/response — un cliente JWKS estándar espera
// `{"keys": [...]}` en la raíz.
//
// Cache-Control de 5 min: muy por debajo de la gracia de rotación (≥ TTL del
// access token, 15 min, ADR-003 §c), así que un consumidor con cache ve la
// clave nueva antes de que se retire la vieja.
func JWKSHandler(p JWKSProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=300")
		c.JSON(http.StatusOK, p.PublicJWKS())
	}
}
