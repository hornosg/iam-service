package value_object

import "fmt"

type UserStatus string

const (
	StatusActive   UserStatus = "ACTIVE"
	StatusInactive UserStatus = "INACTIVE"
	StatusPending  UserStatus = "PENDING"
	StatusBlocked  UserStatus = "BLOCKED"
	StatusDeleted  UserStatus = "DELETED"
)

func NewUserStatusFromString(s string) (UserStatus, error) {
	status := UserStatus(s)
	if !status.IsValid() {
		return "", fmt.Errorf("invalid user status: %s", s)
	}
	return status, nil
}

func (s UserStatus) IsValid() bool {
	switch s {
	case StatusActive, StatusInactive, StatusPending, StatusBlocked, StatusDeleted:
		return true
	default:
		return false
	}
}

func (s UserStatus) String() string {
	return string(s)
}
