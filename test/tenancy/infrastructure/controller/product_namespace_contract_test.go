package controller_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/hornosg/iam-service/src/shared/validator"
	"github.com/stretchr/testify/require"
)

// TEST-T9-HTTP: el namespace de producto viene sólo del body en create.
// No prueba autenticación: simula un claim ya verificado, sin cambiar sus gates.
func TestT9_CreateNamespaceDoesNotComeFromTokenOrQuery(t *testing.T) {
	for _, tc := range []struct {
		name, claim, bodyNamespace, want string
	}{
		{"omitted-with-other-product-claim", "whatsapp-agent", "", "mercado-cercano"},
		{"explicit-with-conflicting-product-claim", "whatsapp-agent", "mercado-cercano", "mercado-cercano"},
		{"explicit-with-token-alias", "mc", "whatsapp-agent", "whatsapp-agent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: usar registro real del handler, body legado sin namespace cuando se omite.
			validator.RegisterCustomValidators()
			repo := newHandlerRepo()
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("jwt_claims", jwt.MapClaims{"namespace": tc.claim})
				c.Next()
			})
			tenantHandler(repo).RegisterProvisionRoutes(router.Group("/api/v1"))
			body := map[string]string{"name": "Store", "slug": "same", "description": "Description", "type": "BUSINESS", "owner_id": uuid.NewString()}
			if tc.bodyNamespace != "" {
				body["namespace"] = tc.bodyNamespace
			}
			payload, err := json.Marshal(body)
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants?namespace=query-product", strings.NewReader(string(payload)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// Act.
			router.ServeHTTP(w, req)

			// Assert: si se deriva del claim o query, falla incluso con namespace omitido.
			require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
			require.NotNil(t, repo.created)
			require.Equal(t, tc.want, repo.created.Namespace)
			require.Equal(t, tc.want, repo.namespace)
			var got map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			require.Equal(t, tc.want, got["namespace"])
			require.Equal(t, repo.created.ID.String(), got["id"])
		})
	}
}
