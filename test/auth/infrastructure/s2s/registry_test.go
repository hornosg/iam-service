package s2s_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"iam/src/auth/infrastructure/s2s"
)

// registry.go es lógica pura de scopes S2S (sin DB): carga de env, lookup
// constant-time y verificación de scopes. Estos tests la cubren sin dobles.

// --- LoadFromEnv ---

func TestLoadFromEnv_BuildsRegistryFromEnv(t *testing.T) {
	t.Setenv("S2S_KEY_WHATSAPP_AGENT", "key-whatsapp-agent-0123456789abcdef")
	t.Setenv("S2S_KEY_ONBOARDING", "key-onboarding-0123456789abcdef")

	r, err := s2s.LoadFromEnv()
	assert.NoError(t, err)

	// whatsapp-agent -> tenant:provision
	if cred, ok := r.Lookup("key-whatsapp-agent-0123456789abcdef"); assert.True(t, ok) {
		assert.Equal(t, "whatsapp-agent", cred.Service)
		assert.True(t, cred.HasScope(s2s.ScopeTenantProvision))
		assert.False(t, cred.HasScope(s2s.ScopeSystemAdmin))
	}
	// onboarding -> provision + admin (dos scopes), no system:admin
	if cred, ok := r.Lookup("key-onboarding-0123456789abcdef"); assert.True(t, ok) {
		assert.Equal(t, "onboarding", cred.Service)
		assert.True(t, cred.HasScope(s2s.ScopeTenantProvision))
		assert.True(t, cred.HasScope(s2s.ScopeTenantAdmin))
		assert.False(t, cred.HasScope(s2s.ScopeSystemAdmin))
	}
}

func TestLoadFromEnv_EmptyKeySkipped(t *testing.T) {
	// Solo seteamos whatsapp-agent; onboarding queda sin key -> no se agrega.
	t.Setenv("S2S_KEY_WHATSAPP_AGENT", "key-whatsapp-agent-0123456789abcdef")

	r, err := s2s.LoadFromEnv()
	assert.NoError(t, err)

	// El servicio sin key no es encontrable con ningún valor.
	_, ok := r.Lookup("")
	assert.False(t, ok)
	// Y whatsapp-agent sí está.
	_, ok = r.Lookup("key-whatsapp-agent-0123456789abcdef")
	assert.True(t, ok)
}

func TestLoadFromEnv_ShortKeyWarnsButStillRegisters(t *testing.T) {
	// Una key más corta que minS2SKeyBytes (16) se registra igual pero loguea
	// un warning a stderr. Capturamos stderr para verificar la advertencia
	// sin romper el boot (fail-closed no aplica a longitudes). Usamos un valor
	// de key ("abcxyz") que no aparece como subcadena en el texto del warning,
	// para poder afirmar que el warning no filtra la key.
	t.Setenv("S2S_KEY_SALES", "abcxyz")

	// Redirigir stderr para atrapar el warning (no loguea la key).
	orig := os.Stderr
	rErr, wErr, _ := os.Pipe()
	os.Stderr = wErr

	r, err := s2s.LoadFromEnv()

	wErr.Close()
	os.Stderr = orig
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(rErr)

	assert.NoError(t, err)
	// La key corta igual se registra.
	if cred, ok := r.Lookup("abcxyz"); assert.True(t, ok) {
		assert.Equal(t, "sales", cred.Service)
		assert.True(t, cred.HasScope(s2s.ScopeSystemAdmin))
	}
	// El warning nombra al servicio y la longitud, pero NO la key.
	assert.Contains(t, buf.String(), "sales")
	assert.Contains(t, buf.String(), "shorter than")
	assert.NotContains(t, buf.String(), "abcxyz")
}

func TestLoadFromEnv_NoEnvYieldsEmptyRegistry(t *testing.T) {
	// Sin ninguna S2S_KEY_* el registro queda vacío (fail-closed). Lookup
	// de cualquier key devuelve false.
	r, err := s2s.LoadFromEnv()
	assert.NoError(t, err)
	_, ok := r.Lookup("anything")
	assert.False(t, ok)
}

// --- Lookup ---

func TestLookup_NilRegistry(t *testing.T) {
	var r *s2s.Registry
	cred, ok := r.Lookup("whatever")
	assert.False(t, ok)
	assert.Nil(t, cred)
}

func TestLookup_NoMatch(t *testing.T) {
	r := s2s.LoadFromEnvForTests(map[string]string{
		"whatsapp-agent": "key-wa",
	})
	cred, ok := r.Lookup("not-a-key")
	assert.False(t, ok)
	assert.Nil(t, cred)
}

func TestLookup_MatchReturnsCredential(t *testing.T) {
	r := s2s.LoadFromEnvForTests(map[string]string{
		"whatsapp-agent": "key-wa",
	})
	cred, ok := r.Lookup("key-wa")
	assert.True(t, ok)
	assert.Equal(t, "whatsapp-agent", cred.Service)
}

// --- HasScope / HasAnyScope ---

func TestHasScopeAndHasAnyScope(t *testing.T) {
	cred := &s2s.Credential{
		Service: "onboarding",
		Scopes:  []s2s.Scope{s2s.ScopeTenantProvision, s2s.ScopeTenantAdmin},
	}

	// HasScope: presente y ausente.
	assert.True(t, cred.HasScope(s2s.ScopeTenantProvision))
	assert.True(t, cred.HasScope(s2s.ScopeTenantAdmin))
	assert.False(t, cred.HasScope(s2s.ScopeSystemAdmin))

	// HasAnyScope: al menos uno presente, o ninguno.
	assert.True(t, cred.HasAnyScope([]s2s.Scope{s2s.ScopeSystemAdmin, s2s.ScopeTenantAdmin}))
	assert.False(t, cred.HasAnyScope([]s2s.Scope{s2s.ScopeSystemAdmin}))
	// Requerido vacío -> false (no tiene ningún scope mágico).
	assert.False(t, cred.HasAnyScope(nil))

	// Credencial sin scopes.
	empty := &s2s.Credential{Service: "x"}
	assert.False(t, empty.HasScope(s2s.ScopeTenantAdmin))
	assert.False(t, empty.HasAnyScope([]s2s.Scope{s2s.ScopeTenantAdmin}))
}

// --- ServiceNameFromEnv ---

func TestServiceNameFromEnv(t *testing.T) {
	cases := []struct {
		name    string
		envVar  string
		want    string
		wantErr bool
	}{
		{name: "canonical upper", envVar: "S2S_KEY_WHATSAPP_AGENT", want: "whatsapp-agent"},
		{name: "onboarding", envVar: "S2S_KEY_ONBOARDING", want: "onboarding"},
		{name: "not an S2S key", envVar: "OTHER_VAR", wantErr: true},
		{name: "empty after prefix", envVar: "S2S_KEY_", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s2s.ServiceNameFromEnv(tc.envVar)
			if tc.wantErr {
				assert.Error(t, err)
				assert.Empty(t, got)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

// --- LoadFromEnvForTests (rama faltante: key vacía explícita) ---

func TestLoadFromEnvForTests_EmptyAndMissingKeys(t *testing.T) {
	// "onboarding" con key vacía se ignora; "pim" ausente se ignora.
	// "sales" con key válida se registra.
	r := s2s.LoadFromEnvForTests(map[string]string{
		"onboarding": "",
		"sales":      "key-sales",
	})
	_, ok := r.Lookup("")
	assert.False(t, ok)
	if cred, ok := r.Lookup("key-sales"); assert.True(t, ok) {
		assert.Equal(t, "sales", cred.Service)
	}
}

// --- normalizeEnvName (vía LoadFromEnv: whatsapp-agent -> S2S_KEY_WHATSAPP_AGENT) ---

func TestNormalizeEnvName_ViaLoadFromEnv(t *testing.T) {
	// Si normalizeEnvName funciona, la key de whatsapp-agent se encuentra
	// seteando S2S_KEY_WHATSAPP_AGENT (guiones -> underscores, uppercase).
	t.Setenv("S2S_KEY_WHATSAPP_AGENT", "key-wa-normalized")
	r, err := s2s.LoadFromEnv()
	assert.NoError(t, err)
	_, ok := r.Lookup("key-wa-normalized")
	assert.True(t, ok)

	// Y un nombre sin guiones (onboarding) también resuelve.
	t.Setenv("S2S_KEY_ONBOARDING", "key-onb-normalized")
	// Recargar para que pise la env nueva.
	r2, err := s2s.LoadFromEnv()
	assert.NoError(t, err)
	_, ok = r2.Lookup("key-onb-normalized")
	assert.True(t, ok)
	// sanity del helper de strings
	assert.True(t, strings.Contains("S2S_KEY_WHATSAPP_AGENT", "WHATSAPP"))
}