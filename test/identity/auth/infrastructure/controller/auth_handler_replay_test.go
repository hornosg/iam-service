package controller_test

// ACC-E08 T3 (fase TEST — QA): caso de replay del refresh token (técnica de misuse
// #15). La rotación es un control: si RefreshTokenUseCase no borrara el token usado,
// un refresh token capturado podría reutilizarse indefinidamente. Este test falla si
// el token ya consumido vuelve a funcionar.

import (
	jsonStd "encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hornosg/iam-service/src/identity/domain/port"
	authrepo "github.com/hornosg/iam-service/test/identity/auth/infrastructure/persistence/repository"
)

func TestAuthRoutes_Refresh_ReplayedToken_Unauthorized(t *testing.T) {
	// Primera rotación OK. Reusar el MISMO refresh token (replay) debe dar 401:
	// el usecase lo borró al rotar, GetRefreshToken ya no lo encuentra.
	userID, tenantID := uuid.New(), uuid.New()
	userSvc := NewMockUserService()
	userSvc.SetupUser(&port.UserData{ID: userID, Email: "replay@test.com", TenantID: tenantID,
		RoleID: uuid.New(), Status: "ACTIVE"})
	mockRepo := authrepo.NewMockAuthRepository()
	seedRefreshToken(mockRepo, userID, tenantID, "replay_refresh_token", time.Now().Add(time.Hour))
	router := newAuthRouter(newAuthHandler(mockRepo, nil, userSvc, nil))

	primera := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/refresh",
		map[string]interface{}{"refresh_token": "replay_refresh_token"}, nil)
	require.Equal(t, http.StatusOK, primera.Code)
	require.Equal(t, 1, mockRepo.GetCallCount("DeleteRefreshToken"))

	var primeraResp map[string]interface{}
	require.NoError(t, jsonUnmarshal(primera.Body.Bytes(), &primeraResp))
	require.NotEmpty(t, primeraResp["refresh_token"]) // rotación: sale uno NUEVO

	replay := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/refresh",
		map[string]interface{}{"refresh_token": "replay_refresh_token"}, nil)

	assert.Equal(t, http.StatusUnauthorized, replay.Code)
	assert.Contains(t, replay.Body.String(), "Refresh token inválido")
	// El UNICO DeleteRefreshToken es la rotación del refresh exitoso: el replay
	// aborta ANTES, en GetRefreshToken (refresh_token.go:67 — no hay fila que
	// borrar), así que no hay segunda rotación que emita credenciales nuevas.
	assert.Equal(t, 1, mockRepo.GetCallCount("DeleteRefreshToken"))
}

// jsonUnmarshal envuelve json.Unmarshal para no importar encoding/json dos veces
// con alias distintos entre archivos del paquete.
func jsonUnmarshal(data []byte, v interface{}) error {
	return jsonStd.Unmarshal(data, v)
}