package s2s

import (
	"crypto/subtle"
	"fmt"
	"os"
	"strings"
)

// Scope son los permisos S2S posibles. Son una política (no secreta) que declara
// qué puede hacer cada servicio consumidor.
type Scope string

const (
	// ScopeTenantProvision permite crear tenants y sus usuarios owner iniciales.
	// Caso de uso: whatsapp-agent en el wizard de onboarding.
	ScopeTenantProvision Scope = "tenant:provision"
	// ScopeTenantAdmin permite operaciones tenant-scoped (users, roles) sin ser
	// cross-tenant. Es el scope S2S mínimo para servicios que administran
	// contenido dentro de un tenant.
	ScopeTenantAdmin Scope = "tenant:admin"
	// ScopeSystemAdmin da acceso equivalente a system_admin sobre los endpoints
	// de gestión del IAM. Es el scope más amplio; solo onboarding/sales/pim legacy.
	ScopeSystemAdmin Scope = "system:admin"
)

// Credential vincula una API key (secreta, vive en env) con un servicio y sus scopes.
type Credential struct {
	Service string
	Scopes  []Scope
}

// Registry mantiene en memoria el mapeo API key -> Credential. Es read-only después
// del boot: el diseño permite rotar una key sin tocar las demás simplemente
// cambiando la env var y reiniciando el pod.
type Registry struct {
	entries []entry
}

type entry struct {
	key        []byte
	credential Credential
}

// ServicePolicy declara, en código/config del repo (no secreto), qué scopes tiene
// cada servicio consumidor. La key en sí viene de S2S_KEY_<SERVICE> en runtime.
var ServicePolicy = map[string][]Scope{
	"whatsapp-agent": {ScopeTenantProvision},
	// onboarding-service: gestión tenant-scoped (provision + admin). Ya no requiere
	// system:admin; los endpoints por tenant en IAM fueron movidos a tenant-scoped.
	"onboarding": {ScopeTenantProvision, ScopeTenantAdmin},
	// Consumidores legacy que hoy usan la god-key compartida. Se les asigna
	// system:admin para no romperlos durante la migración, pero queda marcado
	// como legacy y debe revisarse en una épica de hardening.
	"sales": {ScopeSystemAdmin},
	"pim":   {ScopeSystemAdmin},
}

// minS2SKeyBytes es la longitud mínima aceptable para una S2S key. 32 bytes
// hex (256 bits) es el estándar recomendado por la app. Keys más cortas loguean
// un warning pero no bloquean el boot, para no romper entornos de test con keys
// cortas. En producción, todas las keys deben cumplir este mínimo.
const minS2SKeyBytes = 16

// LoadFromEnv construye el registro cruzando ServicePolicy con las variables
// de entorno S2S_KEY_<SERVICE>. Keys vacías o servicios sin política se ignoran
// (fail-closed). El prefijo S2S_KEY_ evita colisiones y hace explícito el origen.
func LoadFromEnv() (*Registry, error) {
	var r Registry
	for service, scopes := range ServicePolicy {
		key := os.Getenv("S2S_KEY_" + normalizeEnvName(service))
		if key == "" {
			continue
		}
		if len(key) < minS2SKeyBytes {
			// No logueamos la key; solo advertimos por servicio.
			fmt.Fprintf(os.Stderr, "warning: S2S key for service %q is shorter than %d bytes; rotate it\n", service, minS2SKeyBytes)
		}
		r.entries = append(r.entries, entry{
			key: []byte(key),
			credential: Credential{
				Service: service,
				Scopes:  scopes,
			},
		})
	}
	return &r, nil
}

// LoadFromEnvForTests permite tests inyectar credenciales sin tocar el entorno.
func LoadFromEnvForTests(creds map[string]string) *Registry {
	var r Registry
	for service, scopes := range ServicePolicy {
		key, ok := creds[service]
		if !ok || key == "" {
			continue
		}
		r.entries = append(r.entries, entry{
			key: []byte(key),
			credential: Credential{
				Service: service,
				Scopes:  scopes,
			},
		})
	}
	return &r
}

// Lookup resuelve una API key contra el registro usando comparación constant-time
// sobre todas las candidatas. Esto evita oráculos de timing que revelen cuál
// servicio envió una key parcialmente válida.
// Devuelve (nil, false) si no hay match.
func (r *Registry) Lookup(provided string) (*Credential, bool) {
	if r == nil {
		return nil, false
	}
	providedBytes := []byte(provided)
	for _, e := range r.entries {
		if subtle.ConstantTimeCompare(providedBytes, e.key) == 1 {
			return &e.credential, true
		}
	}
	return nil, false
}

// HasScope verifica si una credencial posee el scope requerido.
func (c *Credential) HasScope(required Scope) bool {
	for _, s := range c.Scopes {
		if s == required {
			return true
		}
	}
	return false
}

// HasAnyScope verifica si una credencial posee al menos uno de los scopes requeridos.
func (c *Credential) HasAnyScope(required []Scope) bool {
	for _, r := range required {
		if c.HasScope(r) {
			return true
		}
	}
	return false
}

// normalizeEnvName convierte nombres de servicio con guiones a underscores para
// nombres de variables de entorno: whatsapp-agent -> S2S_KEY_WHATSAPP_AGENT.
func normalizeEnvName(service string) string {
	return strings.ToUpper(strings.ReplaceAll(service, "-", "_"))
}

// ServiceNameFromEnv devuelve el nombre canónico del servicio dado una variable
// de entorno S2S_KEY_<NAME>. Útil para logs que NO loguean la key.
func ServiceNameFromEnv(envVar string) (string, error) {
	const prefix = "S2S_KEY_"
	if !strings.HasPrefix(strings.ToUpper(envVar), prefix) {
		return "", fmt.Errorf("not an S2S key env var: %s", envVar)
	}
	name := strings.ToLower(strings.TrimPrefix(envVar, prefix))
	return strings.ReplaceAll(name, "_", "-"), nil
}
