package exception

import "errors"

var (
	ErrPlanNotFound      = errors.New("plan not found")
	ErrPlanAlreadyExists = errors.New("plan already exists")
	ErrInvalidPlanType   = errors.New("invalid plan type")
	ErrInvalidPlanStatus = errors.New("invalid plan status")
	ErrPlanNotActive     = errors.New("plan is not active")
	ErrInvalidPricing    = errors.New("invalid pricing")
	ErrFeatureNotFound   = errors.New("feature not found")
)
