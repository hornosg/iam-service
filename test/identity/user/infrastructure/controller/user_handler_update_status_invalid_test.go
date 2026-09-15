package controller_test

// ACC-E08 T3 (fase TEST — QA): caso que pinza el contrato REAL del status inválido en
// UpdateUser. Premisa descartada por medición: NO llega 400 "Estado inválido".
// HasStatusUpdate() (update_user_request.go:24-26) exige IsValid() ANTES de que el
// usecase llame a ChangeStatus, así que un status inválido no se rechaza: se DESCARTA
// en silencio y el usecase guarda igual (update_user.go:53-57 no entra; 60 sí) → 200.
// El branch ErrInvalidStatus del handler (user_handler.go:166) es inalcanzable vía HTTP
// por el mismo filtro — coincide con lo documentado por la entrada EXEC de T3.
// El descarte silencioso es una anomalía de contrato registrada en la bitácora de
// ACC-E08 (el cliente cree que actualizó y no pasó nada): corregirlo es un cambio en
// identity/application, fuera del alcance de esta campaña de cobertura.

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	domentity "github.com/hornosg/iam-service/src/identity/domain/entity"
	usermother "github.com/hornosg/iam-service/test/identity/user/domain/entity"
	userrepo "github.com/hornosg/iam-service/test/identity/user/infrastructure/persistence/repository"
)

func TestUserRoutes_Update_InvalidStatus_SilentlyIgnored_Ok(t *testing.T) {
	// Status que no es ninguno de los cinco válidos (ACTIVE/INACTIVE/PENDING/
	// BLOCKED/DELETED): HasStatusUpdate() lo filtra, el usecase guarda sin tocar
	// el status y el handler responde 200 con el usuario SIN CAMBIOS de status.
	// El resto del payload se deja sin email a propósito: la única actualización
	// pedida es la inválida.
	user := usermother.UserMother{}.WithDefaults()
	mockRepo := userrepo.NewMockUserRepository()
	mockRepo.SetupUsers([]*domentity.User{user})
	router := newUserRouter(mockRepo)

	w := doUserRequest(t, router, "PUT", "/api/v1/users/"+user.ID.String(),
		map[string]interface{}{"status": "NO-ES-UN-ESTADO"}, nil)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"status":"ACTIVE"`) // el status quedó intacto
	// El usecase SÍ guarda (una vez): el descarte del campo es a nivel de caso de uso.
	assert.Equal(t, 1, mockRepo.GetCallCount("Update"))
	// Oráculo negativo: ni el 500 del default ni el 400 de la rama inalcanzable.
	assert.NotContains(t, w.Body.String(), "Error interno del servidor")
	assert.NotContains(t, w.Body.String(), "Estado inválido")
}