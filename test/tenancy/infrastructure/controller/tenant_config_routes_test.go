package controller_test

// ACC-E04 T4 — rutas de config con el gate REAL de DD-1 (Authorize + registry con la
// ServicePolicy de producción) y el adapter REAL de auditoría (go-shared
// NewCanonicalLoggerWithWriter). El repo es mock: el aislamiento RLS en DB real lo
// prueban test/tenancy/integration (T3) y el smoke vivo de T4.

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"github.com/hornosg/iam-service/src/identity/infrastructure/adapter"
	"github.com/hornosg/iam-service/src/identity/infrastructure/config"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	sharedlog "github.com/hornosg/go-shared/infrastructure/logging"

	authmw "github.com/hornosg/iam-service/src/access/infrastructure/middleware"
	"github.com/hornosg/iam-service/src/access/infrastructure/s2s"
	"github.com/hornosg/iam-service/src/tenancy/application/usecase"
	"github.com/hornosg/iam-service/src/tenancy/domain/entity"
	"github.com/hornosg/iam-service/src/tenancy/infrastructure/audit"
	"github.com/hornosg/iam-service/src/tenancy/infrastructure/controller"
)

const (
	cfgJWTSecret   = "t4-jwt-secret-SENTINEL-JWT-0123456789"
	cfgNamespace   = "mc"
	catalogBFFKey  = "t4-catalog-bff-SENTINEL-KEY-0123456789abcdef"
	whatsappCfgKey = "t4-whatsapp-SENTINEL-KEY-0123456789abcdef"
	onboardingKey  = "t4-onboarding-SENTINEL-KEY-0123456789abcdef"
	unknownKey     = "t4-unknown-SENTINEL-KEY-0123456789abcdef"
	sentinelValue  = "SENTINEL_CONFIG_VALUE"
	sentinelErr    = "SENTINEL_RAW_DB_ERROR"
	sentinelKey    = "catalog.SENTINEL_secret_key_name"
)

// sentinels no pueden aparecer jamás en una línea de auditoría.
var sentinels = []string{"SENTINEL", catalogBFFKey, whatsappCfgKey, onboardingKey, unknownKey,
	sentinelValue, sentinelErr, sentinelKey, cfgJWTSecret, "Bearer", "evil-service"}

type configRig struct {
	router *gin.Engine
	repo   *mockConfigRepo
	audit  *bytes.Buffer
}

func newConfigRig(t *testing.T, w io.Writer) *configRig {
	t.Helper()
	repo := new(mockConfigRepo)
	buf := &bytes.Buffer{}
	if w == nil {
		w = buf
	}
	emitter := sharedlog.NewCanonicalLoggerWithWriter("iam", w)
	registry := s2s.LoadFromEnvForTests(map[string]string{
		"catalog-bff":    catalogBFFKey,
		"whatsapp-agent": whatsappCfgKey,
		"onboarding":     onboardingKey,
	})
	factory := authmw.NewScopeMiddlewareFactory(cfgNamespace, registry, cfgVerifier())
	ctrl := controller.NewTenantConfigController(
		usecase.NewGetTenantConfigUseCase(repo),
		usecase.NewSetTenantConfigUseCase(repo),
		usecase.NewBootstrapTenantConfigUseCase(repo),
	)
	r := gin.New()
	r.Use(gin.Recovery())
	v1 := r.Group("/api/v1")
	// Misma cadena que SetupTenantConfigModule (router.go): auditoría → gate → handler.
	ctrl.RegisterReadRoute(v1, audit.ConfigAccess(emitter, audit.OperationRead), factory.RequireScope(s2s.ScopeConfigRead))
	ctrl.RegisterSetRoute(v1, factory.RequireScopes(nil, "tenant_admin"))
	ctrl.RegisterBootstrapRoute(v1, audit.ConfigAccess(emitter, audit.OperationBootstrap), factory.RequireScope(s2s.ScopeTenantProvision))
	return &configRig{router: r, repo: repo, audit: buf}
}

func (rig *configRig) do(method, path string, headers map[string]string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	rig.router.ServeHTTP(w, req)
	return w
}

// singleAuditLine parsea la única línea JSON emitida y verifica envelope, campos
// permitidos y ausencia de centinelas.
func singleAuditLine(t *testing.T, raw string) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	require.Len(t, lines, 1, "una emisión por request completada: %q", raw)
	for _, s := range sentinels {
		assert.NotContains(t, lines[0], s, "la auditoría filtró un dato prohibido")
	}
	var ev map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &ev))
	allowed := map[string]bool{"ts": true, "level": true, "service": true, "event": true,
		"s2s_service": true, "tenant_id": true, "operation": true, "outcome": true, "correlation_id": true}
	for k := range ev {
		assert.True(t, allowed[k], "campo fuera de la allowlist: %s", k)
	}
	assert.Equal(t, "iam", ev["service"])
	assert.Equal(t, audit.Event, ev["event"])
	_, err := time.Parse(time.RFC3339, ev["ts"].(string))
	assert.NoError(t, err)
	_, err = uuid.Parse(ev["correlation_id"].(string))
	assert.NoError(t, err, "correlation_id debe ser UUID generado localmente")
	return ev
}

func TestConfigRoutes_ReadAuditedOutcomes(t *testing.T) {
	tenantA := uuid.New()
	readPath := "/api/v1/tenant/config/" + sentinelKey
	spoof := map[string]string{"X-Service-Name": "evil-service", "s2s_service": "evil-service", "X-Correlation-ID": "SENTINEL-corr"}
	with := func(base map[string]string, extra map[string]string) map[string]string {
		out := map[string]string{}
		for k, v := range spoof {
			out[k] = v
		}
		for k, v := range base {
			out[k] = v
		}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}

	cases := []struct {
		name       string
		headers    map[string]string
		setup      func(r *mockConfigRepo)
		status     int
		outcome    string
		level      string
		service    string
		withTenant bool
	}{
		{"200 con config:read", with(map[string]string{"X-API-Key": catalogBFFKey, "X-Tenant-ID": tenantA.String()}, nil),
			func(r *mockConfigRepo) {
				r.On("GetByKey", mock.Anything, tenantA, sentinelKey).Return(entity.NewTenantConfig(tenantA, sentinelKey, sentinelValue), true, nil)
			}, http.StatusOK, audit.OutcomeSuccess, "info", "catalog-bff", true},
		{"404 key inexistente o de otro tenant", with(map[string]string{"X-API-Key": catalogBFFKey, "X-Tenant-ID": tenantA.String()}, nil),
			func(r *mockConfigRepo) {
				r.On("GetByKey", mock.Anything, tenantA, sentinelKey).Return(nil, false, nil)
			}, http.StatusNotFound, audit.OutcomeNotFound, "info", "catalog-bff", true},
		{"400 tenant uuid.Nil", with(map[string]string{"X-API-Key": catalogBFFKey, "X-Tenant-ID": uuid.Nil.String()}, nil),
			nil, http.StatusBadRequest, audit.OutcomeInvalidRequest, "warn", "catalog-bff", false},
		{"400 tenant no-UUID", with(map[string]string{"X-API-Key": catalogBFFKey, "X-Tenant-ID": "SENTINEL-not-a-uuid"}, nil),
			nil, http.StatusBadRequest, audit.OutcomeInvalidRequest, "warn", "catalog-bff", false},
		{"401 sin credencial", with(map[string]string{"X-Tenant-ID": tenantA.String()}, nil),
			nil, http.StatusUnauthorized, audit.OutcomeDenied, "warn", audit.Unattributed, false},
		{"401 key desconocida + bearer basura", with(map[string]string{"X-API-Key": unknownKey, "Authorization": "Bearer SENTINEL.jwt.garbage", "X-Tenant-ID": tenantA.String()}, nil),
			nil, http.StatusUnauthorized, audit.OutcomeDenied, "warn", audit.Unattributed, false},
		{"403 key válida sin config:read", with(map[string]string{"X-API-Key": onboardingKey, "X-Tenant-ID": tenantA.String()}, nil),
			nil, http.StatusForbidden, audit.OutcomeDenied, "warn", audit.Unattributed, false},
		{"500 fallo de DB", with(map[string]string{"X-API-Key": catalogBFFKey, "X-Tenant-ID": tenantA.String()}, nil),
			func(r *mockConfigRepo) {
				r.On("GetByKey", mock.Anything, tenantA, sentinelKey).Return(nil, false, errors.New(sentinelErr))
			}, http.StatusInternalServerError, audit.OutcomeError, "error", "catalog-bff", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newConfigRig(t, nil)
			if tc.setup != nil {
				tc.setup(rig.repo)
			}
			w := rig.do(http.MethodGet, readPath, tc.headers, nil)
			assert.Equal(t, tc.status, w.Code, w.Body.String())
			if tc.status == http.StatusOK {
				var body map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
				assert.Equal(t, sentinelValue, body["value"])
			}
			if tc.setup == nil {
				rig.repo.AssertNotCalled(t, "GetByKey", mock.Anything, mock.Anything, mock.Anything)
			}
			rig.repo.AssertExpectations(t)

			ev := singleAuditLine(t, rig.audit.String())
			assert.Equal(t, audit.OperationRead, ev["operation"])
			assert.Equal(t, tc.outcome, ev["outcome"])
			assert.Equal(t, tc.level, ev["level"])
			assert.Equal(t, tc.service, ev["s2s_service"], "identidad sólo de Authorize, headers falsificados sin efecto")
			if tc.withTenant {
				assert.Equal(t, tenantA.String(), ev["tenant_id"])
			} else {
				assert.NotContains(t, ev, "tenant_id")
			}
		})
	}
}

// ACC-E04 T4 M1: config:read sólo lee el namespace catalog.*; el resto se rechaza antes de la DB.
func TestConfigRoutes_ReadRestrictedToCatalogNamespace(t *testing.T) {
	tenantA := uuid.New()
	for _, key := range []string{"payments.token", "sentinel.other", "nonamespace", "catalogo.x"} {
		t.Run(key, func(t *testing.T) {
			rig := newConfigRig(t, nil)
			w := rig.do(http.MethodGet, "/api/v1/tenant/config/"+key,
				map[string]string{"X-API-Key": catalogBFFKey, "X-Tenant-ID": tenantA.String()}, nil)
			assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
			rig.repo.AssertNotCalled(t, "GetByKey", mock.Anything, mock.Anything, mock.Anything)
			ev := singleAuditLine(t, rig.audit.String())
			assert.Equal(t, audit.OutcomeDenied, ev["outcome"])
			assert.Equal(t, "catalog-bff", ev["s2s_service"])
		})
	}
}

func TestConfigRoutes_BootstrapAudited(t *testing.T) {
	tenantA := uuid.New()
	t.Run("200 tenant:provision", func(t *testing.T) {
		rig := newConfigRig(t, nil)
		rig.repo.On("InsertMissing", mock.Anything, tenantA, mock.Anything).Return(1, nil)
		w := rig.do(http.MethodPost, "/api/v1/tenant/bootstrap", map[string]string{"X-API-Key": whatsappCfgKey, "X-Tenant-ID": tenantA.String()}, nil)
		assert.Equal(t, http.StatusOK, w.Code)
		ev := singleAuditLine(t, rig.audit.String())
		assert.Equal(t, audit.OperationBootstrap, ev["operation"])
		assert.Equal(t, audit.OutcomeSuccess, ev["outcome"])
		assert.Equal(t, "whatsapp-agent", ev["s2s_service"])
		assert.Equal(t, tenantA.String(), ev["tenant_id"])
	})
	t.Run("500 fallo de commit nunca es éxito", func(t *testing.T) {
		rig := newConfigRig(t, nil)
		rig.repo.On("InsertMissing", mock.Anything, tenantA, mock.Anything).Return(0, errors.New(sentinelErr+": commit failed"))
		w := rig.do(http.MethodPost, "/api/v1/tenant/bootstrap", map[string]string{"X-API-Key": whatsappCfgKey, "X-Tenant-ID": tenantA.String()}, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.NotContains(t, w.Body.String(), sentinelErr)
		ev := singleAuditLine(t, rig.audit.String())
		assert.Equal(t, audit.OutcomeError, ev["outcome"])
		assert.Equal(t, "error", ev["level"])
	})
	t.Run("403 config:read no alcanza para bootstrap", func(t *testing.T) {
		rig := newConfigRig(t, nil)
		w := rig.do(http.MethodPost, "/api/v1/tenant/bootstrap", map[string]string{"X-API-Key": catalogBFFKey, "X-Tenant-ID": tenantA.String()}, nil)
		assert.Equal(t, http.StatusForbidden, w.Code)
		rig.repo.AssertNotCalled(t, "InsertMissing", mock.Anything, mock.Anything, mock.Anything)
		ev := singleAuditLine(t, rig.audit.String())
		assert.Equal(t, audit.OutcomeDenied, ev["outcome"])
		assert.Equal(t, audit.Unattributed, ev["s2s_service"])
	})
}

// cfgTestKey firma los tokens humanos del test (ACC-E03 T6: el gate sólo
// verifica RS256 por kid) y su pública alimenta la keyring del gate.
var cfgTestKey = func() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
}()

func cfgVerifier() *adapter.JWTServiceAdapter {
	return adapter.NewJWTServiceAdapter(&adapter.SigningKey{PrivateKey: cfgTestKey, KID: config.SigningKeyKID(&cfgTestKey.PublicKey)})
}

func humanToken(t *testing.T, tenantID uuid.UUID, roles ...string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"user_id": uuid.NewString(), "tenant_id": tenantID.String(), "roles": roles,
		"namespace": cfgNamespace, "exp": time.Now().Add(time.Hour).Unix(),
	})
	tok.Header["kid"] = config.SigningKeyKID(&cfgTestKey.PublicKey)
	s, err := tok.SignedString(cfgTestKey)
	require.NoError(t, err)
	return "Bearer " + s
}

func TestConfigRoutes_SetHumanOnly(t *testing.T) {
	claimTenant, headerTenant := uuid.New(), uuid.New()
	body, _ := json.Marshal(map[string]string{"key": "catalog.stock_policy", "value": "IGNORE_STOCK"})
	ct := map[string]string{"Content-Type": "application/json", "X-Tenant-ID": headerTenant.String()}
	merge := func(extra map[string]string) map[string]string {
		out := map[string]string{}
		for k, v := range ct {
			out[k] = v
		}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}

	t.Run("200 tenant_admin: tenant del claim, X-Tenant-ID ignorado", func(t *testing.T) {
		rig := newConfigRig(t, nil)
		rig.repo.On("GetByKey", mock.Anything, claimTenant, "catalog.stock_policy").Return(nil, false, nil)
		rig.repo.On("Save", mock.Anything, mock.MatchedBy(func(c *entity.TenantConfig) bool { return c.TenantID == claimTenant })).Return(nil)
		w := rig.do(http.MethodPost, "/api/v1/tenant/config", merge(map[string]string{"Authorization": humanToken(t, claimTenant, "tenant_admin")}), body)
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
		rig.repo.AssertExpectations(t)
		assert.Empty(t, rig.audit.String(), "set humano no es una ruta S2S auditada por este wrapper")
	})
	t.Run("401 sin credencial", func(t *testing.T) {
		rig := newConfigRig(t, nil)
		w := rig.do(http.MethodPost, "/api/v1/tenant/config", ct, body)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
	t.Run("403 key S2S válida (no hay escritura S2S)", func(t *testing.T) {
		rig := newConfigRig(t, nil)
		w := rig.do(http.MethodPost, "/api/v1/tenant/config", merge(map[string]string{"X-API-Key": catalogBFFKey}), body)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
	t.Run("403 rol insuficiente", func(t *testing.T) {
		rig := newConfigRig(t, nil)
		w := rig.do(http.MethodPost, "/api/v1/tenant/config", merge(map[string]string{"Authorization": humanToken(t, claimTenant, "viewer")}), body)
		assert.Equal(t, http.StatusForbidden, w.Code)
		rig.repo.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
	})
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("SENTINEL writer down") }

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) / 2, nil }

// Fail-open sólo de auditoría: un sink que falla no cambia auth/RLS/HTTP. El adapter
// no detecta estos errores (Emit retorna void) — no se afirma que lo haga.
func TestConfigRoutes_SinkFailureDoesNotChangeOutcome(t *testing.T) {
	tenantA := uuid.New()
	for name, w := range map[string]io.Writer{"writer error": errWriter{}, "short write": shortWriter{}} {
		t.Run(name, func(t *testing.T) {
			rig := newConfigRig(t, w)
			rig.repo.On("GetByKey", mock.Anything, tenantA, "catalog.stock_policy").Return(entity.NewTenantConfig(tenantA, "catalog.stock_policy", "IGNORE_STOCK"), true, nil)
			ok := rig.do(http.MethodGet, "/api/v1/tenant/config/catalog.stock_policy", map[string]string{"X-API-Key": catalogBFFKey, "X-Tenant-ID": tenantA.String()}, nil)
			assert.Equal(t, http.StatusOK, ok.Code)
			assert.Contains(t, ok.Body.String(), "IGNORE_STOCK")
			denied := rig.do(http.MethodGet, "/api/v1/tenant/config/catalog.stock_policy", map[string]string{"X-Tenant-ID": tenantA.String()}, nil)
			assert.Equal(t, http.StatusUnauthorized, denied.Code)
		})
	}
}

func TestConfigAccess_RequiresEmitter(t *testing.T) {
	assert.Panics(t, func() { audit.ConfigAccess(nil, audit.OperationRead) })
	assert.Panics(t, func() {
		audit.ConfigAccess(sharedlog.NewCanonicalLoggerWithWriter("iam", io.Discard), "config.write")
	})
}
