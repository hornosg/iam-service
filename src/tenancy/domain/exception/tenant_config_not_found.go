package exception

import "fmt"

// TenantConfigNotFound representa el error cuando no se encuentra una configuración
type TenantConfigNotFound struct {
	TenantID string
	Key      string
}

func (e *TenantConfigNotFound) Error() string {
	return fmt.Sprintf("tenant config not found for tenant %s with key %s", e.TenantID, e.Key)
}

// NewTenantConfigNotFound crea una nueva instancia del error
func NewTenantConfigNotFound(tenantID, key string) *TenantConfigNotFound {
	return &TenantConfigNotFound{
		TenantID: tenantID,
		Key:      key,
	}
}
