package controller_test

// ACC-E08 T3 (fase TEST — QA): casos complementarios de contrato del UserHandler que
// la suite principal no cubre. Completan las ramas de traducción HTTP que quedan
// descubiertas y pinzan el control de aislamiento por tenant desde AMBOS lados:
// el ajeno (404 sin tocar el repo, en user_handler_test.go) y el propio (pasa).
// El binding de UpdateUserRequest NO valida formato de email — lo valida el VO en
// el usecase — así que ErrInvalidEmail es alcanzable vía HTTP en update (no en
// create, donde binding:"email" lo corta antes).

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domentity "github.com/hornosg/iam-service/src/identity/domain/entity"
	usermother "github.com/hornosg/iam-service/test/identity/user/domain/entity"
	userrepo "github.com/hornosg/iam-service/test/identity/user/infrastructure/persistence/repository"
)

func TestUserRoutes_Update_InvalidEmailFormat_BadRequest(t *testing.T) {
	// "email" sin arroba pasa el binding (no lleva tag email) y llega al usecase:
	// ToEmail → NewEmail falla → exception.ErrInvalidEmail → 400 "Email inválido".
	// Negativa: si el handler no mapeara ese error, caería al default → 500.
	user := usermother.UserMother{}.WithDefaults()
	mockRepo := userrepo.NewMockUserRepository()
	mockRepo.SetupUsers([]*domentity.User{user})
	router := newUserRouter(mockRepo)

	w := doUserRequest(t, router, http.MethodPut, "/api/v1/users/"+user.ID.String(),
		map[string]interface{}{"email": "email-sin-arroba"}, nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Email inválido")
	assert.Equal(t, 0, mockRepo.GetCallCount("Update")) // no llegó a escribir
}

func TestUserRoutes_GetByID_RepoError_TranslatedNotFound(t *testing.T) {
	// Caracterización del contrato real: GetUserByIDUseCase colapsa CUALQUIER
	// error del repo —incluido un fallo de motor— en exception.ErrUserNotFound
	// (get_user_by_id.go:25), así que el handler traduce 404 y la rama 500 de
	// GetByID es inalcanzable vía HTTP. Lo que este test pinza es la mitad
	// valiosa de esa semántica: un error crudo del motor NO se fuga como 500.
	user := usermother.UserMother{}.WithDefaults()
	mockRepo := userrepo.NewMockUserRepository()
	mockRepo.SetupUsers([]*domentity.User{user})
	mockRepo.ShouldFailOn("GetByID")
	router := newUserRouter(mockRepo)

	w := doUserRequest(t, router, http.MethodGet, "/api/v1/users/"+user.ID.String(), nil, nil)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "Usuario no encontrado")
}

func TestUserRoutes_GetByID_TenantMatch_OK(t *testing.T) {
	// Lado PROPIO del control de aislamiento: X-Tenant-ID coincidente con el
	// tenant del usuario → la consulta pasa y devuelve el dato (200). Si el
	// control se rompiera en dirección rechazar-todos, este test falla.
	user := usermother.UserMother{}.WithDefaults()
	mockRepo := userrepo.NewMockUserRepository()
	mockRepo.SetupUsers([]*domentity.User{user})
	router := newUserRouter(mockRepo)

	w := doUserRequest(t, router, http.MethodGet, "/api/v1/users/"+user.ID.String(), nil,
		map[string]string{"X-Tenant-ID": user.TenantID.String()})

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, user.ID.String(), resp["id"])
}

func TestUserRoutes_Delete_TenantMatch_OK(t *testing.T) {
	// Ídem el anterior pero en el flujo destructivo: con header coincidente el
	// pre-check deja pasar y Delete corre UNA vez. Es la contracara del
	// Delete_TenantMismatch_NotFound (404 + 0 llamadas) de la suite principal.
	user := usermother.UserMother{}.WithDefaults()
	mockRepo := userrepo.NewMockUserRepository()
	mockRepo.SetupUsers([]*domentity.User{user})
	router := newUserRouter(mockRepo)

	w := doUserRequest(t, router, http.MethodDelete, "/api/v1/users/"+user.ID.String(), nil,
		map[string]string{"X-Tenant-ID": user.TenantID.String()})

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, 1, mockRepo.GetCallCount("Delete"))
}