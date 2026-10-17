package controller_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/hornosg/go-shared/criteria"
	sharedmetrics "github.com/hornosg/go-shared/infrastructure/metrics"
	"github.com/stretchr/testify/require"

	"github.com/hornosg/iam-service/src/shared/validator"
	"github.com/hornosg/iam-service/src/tenancy/application/usecase"
	"github.com/hornosg/iam-service/src/tenancy/domain/entity"
	"github.com/hornosg/iam-service/src/tenancy/domain/exception"
	"github.com/hornosg/iam-service/src/tenancy/domain/port"
	vo "github.com/hornosg/iam-service/src/tenancy/domain/value_object"
	"github.com/hornosg/iam-service/src/tenancy/infrastructure/controller"
	tenantcriteria "github.com/hornosg/iam-service/src/tenancy/infrastructure/criteria"
	testrepo "github.com/hornosg/iam-service/test/tenancy/infrastructure/persistence/repository"
)

type handlerRepo struct {
	port.TenantRepository
	tenant                                                 *entity.Tenant
	getErr, createErr, updateErr, errorExists, errorSearch error
	slugExists, domainExists                               bool
	created                                                *entity.Tenant
	namespace                                              string
}

func (r *handlerRepo) GetByID(context.Context, uuid.UUID) (*entity.Tenant, error) {
	return r.tenant, r.getErr
}
func (r *handlerRepo) GetBySlug(_ context.Context, namespace, slug string) (*entity.Tenant, error) {
	r.namespace = namespace
	return r.tenant, r.getErr
}
func (r *handlerRepo) ExistsBySlug(_ context.Context, namespace, slug string) (bool, error) {
	r.namespace = namespace
	return r.slugExists, r.errorExists
}
func (r *handlerRepo) ExistsByDomain(context.Context, string) (bool, error) {
	return r.domainExists, nil
}
func (r *handlerRepo) Create(_ context.Context, tenant *entity.Tenant) error {
	r.created = tenant
	return r.createErr
}
func (r *handlerRepo) Update(context.Context, *entity.Tenant) error { return r.updateErr }
func (r *handlerRepo) SearchByCriteria(context.Context, criteria.Criteria) ([]*entity.Tenant, error) {
	return []*entity.Tenant{r.tenant}, r.errorSearch
}
func (r *handlerRepo) CountByCriteria(context.Context, criteria.Criteria) (int, error) { return 1, nil }

func tenantHandler(r *handlerRepo) *controller.TenantHandler {
	return controller.NewTenantHandler(usecase.NewCreateTenantUseCase(r, sharedmetrics.NoopRecorder{}), usecase.NewGetTenantByIDUseCase(r), usecase.NewGetTenantBySlugUseCase(r), usecase.NewUpdateTenantUseCase(r), usecase.NewDeleteTenantUseCase(r), usecase.NewListTenantsUseCase(r), usecase.NewListTenantsByCriteriaUseCase(r), usecase.NewSetPlanUseCase(r), usecase.NewUpdateTenantFeaturesUseCase(r), tenantcriteria.NewTenantCriteriaBuilder())
}
func callTenant(t *testing.T, repo *handlerRepo, method, path, body string, call func(*controller.TenantHandler, *gin.Context)) *httptest.ResponseRecorder {
	t.Helper()
	validator.RegisterCustomValidators()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: repo.tenant.ID.String()}, {Key: "slug", Value: "same"}}
	// Matches the real Gin claim location; request's namespace must win over token namespace.
	c.Set("jwt_claims", jwt.MapClaims{"namespace": "mc"})
	call(tenantHandler(repo), c)
	return w
}
func newHandlerRepo() *handlerRepo {
	return &handlerRepo{TenantRepository: testrepo.NewMockTenantRepository(), tenant: entity.NewTenant("Store", "same", "Description", vo.TenantTypeBusiness, uuid.New())}
}
func TestTenantHandler_CreateNamespace(t *testing.T) {
	for _, namespace := range []string{"", "whatsapp-agent", "mc", "MC"} {
		t.Run(namespace, func(t *testing.T) {
			repo := newHandlerRepo()
			payload := `{"namespace":"` + namespace + `","name":"Store","slug":"same","description":"Description","type":"BUSINESS","owner_id":"` + uuid.NewString() + `"}`
			w := callTenant(t, repo, http.MethodPost, "/tenants", payload, (*controller.TenantHandler).CreateTenant)
			if namespace == "mc" || namespace == "MC" {
				require.Equal(t, 400, w.Code)
				require.Nil(t, repo.created)
				return
			}
			require.Equal(t, 201, w.Code, w.Body.String())
			expected := namespace
			if expected == "" {
				expected = "mercado-cercano"
			}
			require.Equal(t, expected, repo.created.Namespace)
			require.Equal(t, expected, repo.namespace)
			var response map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			require.Equal(t, expected, response["namespace"])
		})
	}
	for _, tc := range []struct {
		name string
		err  error
		code int
	}{
		{"duplicate-slug", exception.ErrSlugAlreadyExists, 409}, {"duplicate-domain", exception.ErrDomainAlreadyExists, 409}, {"invalid-type", exception.ErrInvalidTenantType, 400}, {"invalid-owner", exception.ErrInvalidOwner, 400}, {"unexpected", exception.ErrCannotChangePlan, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newHandlerRepo()
			repo.createErr = tc.err
			payload := `{"name":"Store","slug":"same","description":"Description","type":"BUSINESS","owner_id":"` + uuid.NewString() + `"}`
			w := callTenant(t, repo, "POST", "/tenants", payload, (*controller.TenantHandler).CreateTenant)
			require.Equal(t, tc.code, w.Code)
		})
	}
	repo := newHandlerRepo()
	require.Equal(t, 400, callTenant(t, repo, "POST", "/tenants", "{", (*controller.TenantHandler).CreateTenant).Code)
}
func TestTenantHandler_GetBySlugNamespace(t *testing.T) {
	for _, namespace := range []string{"", "whatsapp-agent", "mc"} {
		t.Run(namespace, func(t *testing.T) {
			repo := newHandlerRepo()
			w := callTenant(t, repo, "GET", "/not-mounted?namespace="+namespace, "", (*controller.TenantHandler).GetTenantBySlug)
			if namespace == "mc" {
				require.Equal(t, 400, w.Code)
				require.Empty(t, repo.namespace)
			} else {
				require.Equal(t, 200, w.Code)
				if namespace == "" {
					namespace = "mercado-cercano"
				}
				require.Equal(t, namespace, repo.namespace)
			}
		})
	}
	for _, tc := range []struct {
		err  error
		code int
	}{{exception.ErrTenantNotFound, 404}, {exception.ErrCannotChangePlan, 500}} {
		repo := newHandlerRepo()
		repo.getErr = tc.err
		require.Equal(t, tc.code, callTenant(t, repo, "GET", "/not-mounted", "", (*controller.TenantHandler).GetTenantBySlug).Code)
	}
	repo := newHandlerRepo()
	w := callTenant(t, repo, "GET", "/not-mounted", "", func(h *controller.TenantHandler, c *gin.Context) { c.Params = nil; h.GetTenantBySlug(c) })
	require.Equal(t, 400, w.Code)
	// Exercise registrations without mounting GetTenantBySlug in production or this test router.
	routes := gin.New()
	h := tenantHandler(newHandlerRepo())
	g := routes.Group("/api/v1")
	h.RegisterProvisionRoutes(g)
	h.RegisterScopedRoutes(g)
	h.RegisterRoutes(g)
	for _, route := range routes.Routes() {
		require.NotContains(t, route.Path, "by-slug")
	}
}
func TestTenantHandler_ExistingManagementPreservesNamespace(t *testing.T) {
	methods := []struct {
		name, method, body string
		call               func(*controller.TenantHandler, *gin.Context)
	}{
		{"get", "GET", "", (*controller.TenantHandler).GetTenantByID},
		{"update", "PUT", `{"name":"Renamed"}`, (*controller.TenantHandler).UpdateTenant},
		{"delete", "DELETE", "", (*controller.TenantHandler).DeleteTenant},
		{"set-plan", "POST", `{"plan_id":"` + uuid.NewString() + `"}`, (*controller.TenantHandler).SetTenantPlan},
		{"remove-plan", "DELETE", "", (*controller.TenantHandler).RemoveTenantPlan},
		{"features", "PATCH", `{"friends_family":true}`, (*controller.TenantHandler).UpdateTenantFeatures},
	}
	for _, method := range methods {
		t.Run(method.name, func(t *testing.T) {
			repo := newHandlerRepo()
			repo.tenant.Namespace = "whatsapp-agent"
			w := callTenant(t, repo, method.method, "/tenants/id", method.body, method.call)
			expected := 200
			if method.name == "delete" {
				expected = 204
			}
			require.Equal(t, expected, w.Code, w.Body.String())
			require.Equal(t, "whatsapp-agent", repo.tenant.Namespace)
			for _, tc := range []struct {
				err  error
				code int
			}{{exception.ErrTenantNotFound, 404}, {exception.ErrCannotChangePlan, 500}} {
				repo = newHandlerRepo()
				repo.getErr = tc.err
				require.Equal(t, tc.code, callTenant(t, repo, method.method, "/tenants/id", method.body, method.call).Code)
			}
			require.Equal(t, 400, callTenant(t, newHandlerRepo(), method.method, "/tenants/bad", method.body, func(h *controller.TenantHandler, c *gin.Context) {
				c.Params = gin.Params{{Key: "id", Value: "bad"}}
				method.call(h, c)
			}).Code)
			if method.body != "" {
				require.Equal(t, 400, callTenant(t, newHandlerRepo(), method.method, "/tenants/id", "{", method.call).Code)
			}
			if method.name != "get" {
				repo = newHandlerRepo()
				repo.tenant.Delete()
				expected := 403
				if method.name == "features" {
					// Existing use case allows feature updates regardless of status; T9 does not change it.
					expected = 200
				}
				require.Equal(t, expected, callTenant(t, repo, method.method, "/tenants/id", method.body, method.call).Code)
			}
		})
	}
	repo := newHandlerRepo()
	repo.tenant.Namespace = "whatsapp-agent"
	w := callTenant(t, repo, "GET", "/tenants", "", (*controller.TenantHandler).ListTenants)
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), "whatsapp-agent")
	repo.errorSearch = exception.ErrCannotChangePlan
	require.Equal(t, 500, callTenant(t, repo, "GET", "/tenants", "", (*controller.TenantHandler).ListTenants).Code)
}
