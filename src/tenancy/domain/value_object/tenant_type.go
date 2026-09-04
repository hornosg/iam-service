package value_object

import "fmt"

type TenantType string

const (
	TenantTypePersonal   TenantType = "PERSONAL"   // Usuario individual
	TenantTypeStartup    TenantType = "STARTUP"    // Startup pequeña
	TenantTypeBusiness   TenantType = "BUSINESS"   // Empresa mediana
	TenantTypeEnterprise TenantType = "ENTERPRISE" // Empresa grande
)

func NewTenantTypeFromString(s string) (TenantType, error) {
	tenantType := TenantType(s)
	if !tenantType.IsValid() {
		return "", fmt.Errorf("invalid tenant type: %s", s)
	}
	return tenantType, nil
}

func (t TenantType) IsValid() bool {
	switch t {
	case TenantTypePersonal, TenantTypeStartup, TenantTypeBusiness, TenantTypeEnterprise:
		return true
	default:
		return false
	}
}

func (t TenantType) String() string {
	return string(t)
}

func (t TenantType) GetDefaultUserLimit() int {
	switch t {
	case TenantTypePersonal:
		return 1
	case TenantTypeStartup:
		return 10
	case TenantTypeBusiness:
		return 100
	case TenantTypeEnterprise:
		return -1 // Unlimited
	default:
		return 1
	}
}

func (t TenantType) RequiresApproval() bool {
	return t == TenantTypeEnterprise
}
