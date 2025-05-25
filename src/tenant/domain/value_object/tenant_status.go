package value_object

import "fmt"

type TenantStatus string

const (
	TenantStatusActive    TenantStatus = "ACTIVE"
	TenantStatusInactive  TenantStatus = "INACTIVE"
	TenantStatusSuspended TenantStatus = "SUSPENDED"
	TenantStatusDeleted   TenantStatus = "DELETED"
)

func NewTenantStatusFromString(s string) (TenantStatus, error) {
	status := TenantStatus(s)
	if !status.IsValid() {
		return "", fmt.Errorf("invalid tenant status: %s", s)
	}
	return status, nil
}

func (t TenantStatus) IsValid() bool {
	switch t {
	case TenantStatusActive, TenantStatusInactive, TenantStatusSuspended, TenantStatusDeleted:
		return true
	default:
		return false
	}
}

func (t TenantStatus) String() string {
	return string(t)
}

func (t TenantStatus) IsActive() bool {
	return t == TenantStatusActive
}

func (t TenantStatus) CanAccess() bool {
	return t == TenantStatusActive
}

func (t TenantStatus) CanBeModified() bool {
	return t != TenantStatusDeleted
}
