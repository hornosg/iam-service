package controller_test

// ACC-E08 T3 (fase TEST — QA): caso negativo de anti-enumeración en el flujo LOCAL.
// El switch del handler tiene una rama 401 "Usuario no encontrado" (http_handler.go:67-68)
// y loginGoogle sí la usa (testeado en auth_handler_test.go). El control que pinza este
// test es el OTRO lado: loginLocal colapsa email inexistente en ErrInvalidCredentials
// (login.go:158-160) para que la respuesta de "email inexistente" sea indistinguible de
// la de "password incorrecta". Si ese colapso se rompiera (p. ej. alguien "corrige"
// loginLocal para devolver ErrUserNotFound), este test falla: un atacante podría
// listar emails válidos comparando los dos mensajes de 401.

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	authrepo "github.com/hornosg/iam-service/test/identity/auth/infrastructure/persistence/repository"
)

func TestAuthRoutes_Login_Local_UnknownEmail_NoEnumeration(t *testing.T) {
	// Email que no existe en ningún lado + password cualquiera: la respuesta debe
	// ser idéntica a la de credenciales incorrectas — 401 "Credenciales inválidas",
	// NUNCA "Usuario no encontrado" (esa rama es exclusiva del flujo Google).
	router := newAuthRouter(newAuthHandler(authrepo.NewMockAuthRepository(), nil, NewMockUserService(), nil))

	w := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/login", map[string]interface{}{
		"email": "nadie-con-este-email@test.com", "password": "password123", "provider": "LOCAL",
	}, nil)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Credenciales inválidas")
	assert.NotContains(t, w.Body.String(), "Usuario no encontrado") // el oráculo de enumeración no debe aparecer
	assert.NotContains(t, w.Body.String(), "Error interno del servidor")
}