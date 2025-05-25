package exception

import "errors"

var (
	ErrTenantNotFound      = errors.New("tenant not found")
	ErrTenantAlreadyExists = errors.New("tenant already exists")
	ErrSlugAlreadyExists   = errors.New("tenant slug already exists")
	ErrInvalidTenantType   = errors.New("invalid tenant type")
	ErrInvalidTenantStatus = errors.New("invalid tenant status")
	ErrTenantNotActive     = errors.New("tenant is not active")
	ErrTenantExpired       = errors.New("tenant subscription expired")
	ErrTenantSuspended     = errors.New("tenant is suspended")
	ErrTenantDeleted       = errors.New("tenant is deleted")
	ErrUserLimitExceeded   = errors.New("user limit exceeded")
	ErrCannotDeleteTenant  = errors.New("cannot delete tenant")
	ErrInvalidOwner        = errors.New("invalid tenant owner")
	ErrDomainAlreadyExists = errors.New("domain already exists")
	ErrInvalidDomain       = errors.New("invalid domain")
	ErrPlanNotFound        = errors.New("plan not found")
	ErrCannotChangePlan    = errors.New("cannot change plan")
)
