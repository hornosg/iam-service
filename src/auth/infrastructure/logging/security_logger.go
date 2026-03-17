package logging

import (
	"encoding/json"
	"io"
	"os"
	"time"
)

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

type SecurityEvent struct {
	EventType string                 `json:"event_type"`
	Severity  Severity               `json:"severity"`
	UserID    string                 `json:"user_id,omitempty"`
	TenantID  string                 `json:"tenant_id,omitempty"`
	IPAddress string                 `json:"ip_address,omitempty"`
	UserAgent string                 `json:"user_agent,omitempty"`
	Timestamp string                 `json:"timestamp"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

type SecurityLogger struct {
	writer io.Writer
}

func NewSecurityLogger() *SecurityLogger {
	return &SecurityLogger{writer: os.Stdout}
}

func NewSecurityLoggerWithWriter(w io.Writer) *SecurityLogger {
	return &SecurityLogger{writer: w}
}

func (l *SecurityLogger) emit(event SecurityEvent) {
	event.Timestamp = time.Now().UTC().Format(time.RFC3339)
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	data = append(data, '\n')
	_, _ = l.writer.Write(data)
}

func (l *SecurityLogger) LogLoginFailed(email, ipAddress, userAgent, reason string) {
	l.emit(SecurityEvent{
		EventType: "auth.login_failed",
		Severity:  SeverityWarning,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		Details: map[string]interface{}{
			"email":  email,
			"reason": reason,
		},
	})
}

func (l *SecurityLogger) LogLoginSuccess(userID, tenantID, email, ipAddress, userAgent string) {
	l.emit(SecurityEvent{
		EventType: "auth.login_success",
		Severity:  SeverityInfo,
		UserID:    userID,
		TenantID:  tenantID,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		Details: map[string]interface{}{
			"email": email,
		},
	})
}

func (l *SecurityLogger) LogLogout(userID, tenantID, ipAddress string) {
	l.emit(SecurityEvent{
		EventType: "auth.logout",
		Severity:  SeverityInfo,
		UserID:    userID,
		TenantID:  tenantID,
		IPAddress: ipAddress,
	})
}

func (l *SecurityLogger) LogTokenRevoked(userID, tenantID, ipAddress, scope string) {
	l.emit(SecurityEvent{
		EventType: "auth.token_revoked",
		Severity:  SeverityInfo,
		UserID:    userID,
		TenantID:  tenantID,
		IPAddress: ipAddress,
		Details: map[string]interface{}{
			"scope": scope,
		},
	})
}

func (l *SecurityLogger) LogTenantMismatch(userID, jwtTenantID, headerTenantID, ipAddress string) {
	l.emit(SecurityEvent{
		EventType: "auth.tenant_mismatch",
		Severity:  SeverityCritical,
		UserID:    userID,
		IPAddress: ipAddress,
		Details: map[string]interface{}{
			"jwt_tenant_id":    jwtTenantID,
			"header_tenant_id": headerTenantID,
		},
	})
}
