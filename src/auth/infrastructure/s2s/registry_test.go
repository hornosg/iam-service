package s2s

import (
	"testing"
)

func TestRegistryLookup(t *testing.T) {
	r := LoadFromEnvForTests(map[string]string{
		"whatsapp-agent": "key-whatsapp",
		"onboarding":     "key-onboarding",
	})

	cases := []struct {
		name         string
		provided     string
		wantFound    bool
		wantService  string
		wantHasScope Scope
		wantHas      bool
	}{
		{
			name:         "key de whatsapp-agent",
			provided:     "key-whatsapp",
			wantFound:    true,
			wantService:  "whatsapp-agent",
			wantHasScope: ScopeTenantProvision,
			wantHas:      true,
		},
		{
			name:         "key de onboarding con scope tenant:admin",
			provided:     "key-onboarding",
			wantFound:    true,
			wantService:  "onboarding",
			wantHasScope: ScopeTenantAdmin,
			wantHas:      true,
		},
		{
			name:         "key de onboarding con scope tenant:provision",
			provided:     "key-onboarding",
			wantFound:    true,
			wantService:  "onboarding",
			wantHasScope: ScopeTenantProvision,
			wantHas:      true,
		},
		{
			name:         "key de onboarding no tiene system:admin",
			provided:     "key-onboarding",
			wantFound:    true,
			wantService:  "onboarding",
			wantHasScope: ScopeSystemAdmin,
			wantHas:      false,
		},
		{
			name:     "key desconocida",
			provided: "key-unknown",
			wantFound: false,
		},
		{
			name:     "key vacia",
			provided: "",
			wantFound: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cred, found := r.Lookup(tc.provided)
			if found != tc.wantFound {
				t.Fatalf("found = %v, want %v", found, tc.wantFound)
			}
			if !found {
				return
			}
			if cred.Service != tc.wantService {
				t.Errorf("service = %q, want %q", cred.Service, tc.wantService)
			}
			if got := cred.HasScope(tc.wantHasScope); got != tc.wantHas {
				t.Errorf("HasScope(%q) = %v, want %v", tc.wantHasScope, got, tc.wantHas)
			}
		})
	}
}

func TestRegistryLookupConstantTime(t *testing.T) {
	r := LoadFromEnvForTests(map[string]string{
		"whatsapp-agent": "key-whatsapp",
	})

	if _, found := r.Lookup("key-whatsapp"); !found {
		t.Fatal("expected to find exact key")
	}
	if _, found := r.Lookup("key-whatsapp-extra"); found {
		t.Fatal("must not match longer key")
	}
	if _, found := r.Lookup("key-whatsap"); found {
		t.Fatal("must not match partial key")
	}
}

func TestLoadFromEnvIgnoresMissingAndUnmapped(t *testing.T) {
	// sales no está en el mapa de creds, whatsapp-agent sí, unknown-service no tiene política.
	r := LoadFromEnvForTests(map[string]string{
		"whatsapp-agent": "k-wa",
		"unknown-service": "k-unk",
	})

	if _, found := r.Lookup("k-wa"); !found {
		t.Error("expected whatsapp-agent key to be loaded")
	}
	if _, found := r.Lookup("k-unk"); found {
		t.Error("unknown-service should not be loaded because it has no policy")
	}
}

func TestNormalizeEnvName(t *testing.T) {
	if got := normalizeEnvName("whatsapp-agent"); got != "WHATSAPP_AGENT" {
		t.Errorf("normalizeEnvName(whatsapp-agent) = %q, want WHATSAPP_AGENT", got)
	}
}
