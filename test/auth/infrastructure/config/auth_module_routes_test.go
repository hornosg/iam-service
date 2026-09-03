package config_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"iam/src/auth/infrastructure/config"
)

// ACC-E02 T8l (criterio (a)): pín negativo sobre la eliminación de
// GET /auth/validate (T8k, decisión del owner 2026-09-02). No basta con que
// no exista el código: un re-scaffold del módulo auth reintroduciría la ruta
// en silencio, y con ella el defecto que motivó su eliminación — un endpoint
// que respondía 200 valid:true a tokens REVOCADOS y no tenía consumidores
// productivos (verificado en el gate de T8k).
//
// Este test cablea el wiring real del módulo — mismo orden que main.go:
// gate de revocación sobre el grupo padre (T8g), después SetupAuthModule —
// y falla si la ruta vuelve a registrarse por cualquier vía.
//
// Las dependencias llegan en nil a propósito: el REGISTRO de rutas no toca
// la DB; sólo la ejecución de un request lo haría, y este test no ejercita
// handlers de rutas existentes.
func TestAuthModule_ValidateRouteNoExiste(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	apiV1 := router.Group("/api/v1")

	testSecret := "test-secret-0123456789abcdef" // fixture, no secreto real

	authRepoApp := config.SetupTokenRevocationGate(apiV1, nil, nil, testSecret)
	config.SetupAuthModule(apiV1, nil, nil, authRepoApp, nil, nil, nil, config.AuthModuleConfig{
		JWTSecret:          testSecret,
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		Namespace:          "mc",
	})

	t.Run("ninguna ruta registrada contiene auth/validate", func(t *testing.T) {
		var offending []string
		for _, r := range router.Routes() {
			if strings.Contains(r.Path, "auth/validate") {
				offending = append(offending, r.Method+" "+r.Path)
			}
		}
		assert.Empty(t, offending,
			"GET /auth/validate fue eliminado por decisión del owner (ACC-E02 T8k): "+
				"respondía valid:true a tokens revocados y no tenía consumidores. "+
				"Si volvió a aparecer, no es un re-scaffold inocente — revisar la decisión "+
				"con el owner antes de restaurar la ruta. Rutas encontradas: %v", offending)
	})

	t.Run("GET /api/v1/auth/validate responde 404 (ruta inexistente)", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/validate", nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code,
			"la ruta eliminada no debe estar registrada; si responde 401/200/500, "+
				"algo la volvió a registrar")
	})
}