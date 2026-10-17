package value_object

import (
	"fmt"
	"strings"
)

// ConfigKey representa una clave de configuración namespaced (`catalog.stock_policy`).
// Ojo: este namespace es el de la clave de config, NO el product-namespace de tenants (DD-3).
type ConfigKey struct {
	value string
}

// NewConfigKey crea una nueva instancia de ConfigKey
func NewConfigKey(value string) (*ConfigKey, error) {
	if value == "" {
		return nil, fmt.Errorf("config key cannot be empty")
	}

	// Validar formato: debe contener al menos un punto (namespace.key)
	if !strings.Contains(value, ".") {
		return nil, fmt.Errorf("config key must be namespaced (e.g., 'catalog.stock_policy')")
	}

	return &ConfigKey{value: value}, nil
}

// String retorna el valor de la clave
func (ck *ConfigKey) String() string {
	return ck.value
}

// Namespace retorna el namespace de la clave (parte antes del primer punto)
func (ck *ConfigKey) Namespace() string {
	parts := strings.SplitN(ck.value, ".", 2)
	return parts[0]
}

// Key retorna la clave sin el namespace (parte después del primer punto)
func (ck *ConfigKey) Key() string {
	parts := strings.SplitN(ck.value, ".", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}
