package logging

import (
	"io"

	"github.com/hornosg/iam-service/src/identity/domain/port"
	sharedlog "github.com/hornosg/go-shared/infrastructure/logging"
)

// TokenMaintenanceLogger es el adapter canónico para eventos de mantenimiento de tokens (ADR-001).
// Delega en sharedlog.CanonicalLogger para emitir el envelope JSON ts/level/service/event.
// El service se fija en construcción; nunca se pasa por llamada.
type TokenMaintenanceLogger struct {
	canonical *sharedlog.CanonicalLogger
}

// NewTokenMaintenanceLogger crea un logger que escribe a stdout.
func NewTokenMaintenanceLogger(service string) *TokenMaintenanceLogger {
	return &TokenMaintenanceLogger{
		canonical: sharedlog.NewCanonicalLogger(service),
	}
}

// NewTokenMaintenanceLoggerWithWriter permite inyectar un io.Writer (tests: bytes.Buffer / io.Discard).
func NewTokenMaintenanceLoggerWithWriter(service string, w io.Writer) *TokenMaintenanceLogger {
	return &TokenMaintenanceLogger{
		canonical: sharedlog.NewCanonicalLoggerWithWriter(service, w),
	}
}

// Log emite una línea JSON canónica para el evento de mantenimiento de tokens.
func (l *TokenMaintenanceLogger) Log(e port.TokenMaintenanceEvent) {
	level := levelFor(e.Event)
	fields := map[string]any{}

	if e.Count > 0 {
		fields["count"] = e.Count // int64, serializable a JSON number
	}
	if e.Reason != "" {
		fields["reason"] = e.Reason
	}

	l.canonical.Emit(level, string(e.Event), fields)
}

// levelFor determina el nivel según el tipo de evento (reglas ADR-001).
func levelFor(e port.TokenMaintenanceEventType) string {
	switch e {
	case port.EventRevocationCleanupFailed:
		return "error"
	default:
		return "info"
	}
}
