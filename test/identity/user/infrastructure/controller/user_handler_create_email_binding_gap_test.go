package controller_test

// ACC-E08 T3 (fase TEST — QA): caso negativo que pinza la rama ErrInvalidEmail del
// CreateUser handler (user_handler.go:67-68). Es alcanzable vía HTTP porque los dos
// validadores de email no coinciden en la LONGITUD del TLD: el tag `binding:"email"`
// de go-playground (v10.20, regex de regexes.go) exige al menos un punto en el
// dominio (`\.)+`) pero acepta TLD de UN carácter ("usuario@a.b"), mientras el VO
// NewEmail exige TLD de 2+ (`\.[a-zA-Z]{2,}`). Ojo: "sin-tld@test" y
// "usuario@localhost" NO sirven de sonda — el tag `email` los rechaza también y
// caen en el binding sin llegar al VO. El hueco entre ambos validadores es lo que
// deja llegar el error de dominio al handler. Si el mapping no estuviera, el error
// caería a la rama default → 500 con el detalle crudo del VO en el cuerpo
// (JSONWithDetails) — ese es el fallo que este test detecta.

import (
	"testing"

	"github.com/stretchr/testify/assert"

	userrepo "github.com/hornosg/iam-service/test/identity/user/infrastructure/persistence/repository"
)

func TestUserRoutes_Create_EmailWithoutTLD_PassesBinding_BadRequest(t *testing.T) {
	// "usuario@a.b" pasa el binding (go-playground acepta TLD de 1 carácter) pero
	// falla el VO → exception.ErrInvalidEmail → 400 "Email inválido". El repo no
	// debe recibir un Create: el dato inválido se corta en la frontera HTTP.
	mockRepo := userrepo.NewMockUserRepository()
	router := newUserRouter(mockRepo)

	w := doUserRequest(t, router, "POST", "/api/v1/users", map[string]interface{}{
		"email": "usuario@a.b", "password": "password123",
		"tenant_id": "11111111-1111-1111-1111-111111111111",
		"role_id":   "22222222-2222-2222-2222-222222222222",
	}, nil)

	assert.Equal(t, 400, w.Code)
	assert.Contains(t, w.Body.String(), "Email inválido")
	assert.NotContains(t, w.Body.String(), "Error interno del servidor") // si cae al default, el mapping falta
	assert.Equal(t, 0, mockRepo.GetCallCount("Create"))                 // nunca llegó a escribir
}