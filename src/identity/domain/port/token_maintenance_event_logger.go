package port

// TokenMaintenanceEventType identifies a token maintenance event.
// Convention: token.<action>_<result>, lowercase, dot-separated.
type TokenMaintenanceEventType string

const (
	// EventRevocationCleanupCompleted se emite cuando el job de limpieza elimina entradas expiradas.
	EventRevocationCleanupCompleted TokenMaintenanceEventType = "token.revocation_cleanup_completed"
	// EventRevocationCleanupFailed se emite cuando el job de limpieza falla.
	EventRevocationCleanupFailed TokenMaintenanceEventType = "token.revocation_cleanup_failed"
)

// TokenMaintenanceEvent es el payload canónico para eventos del job de mantenimiento de tokens.
// Todos los campos excepto Event son opcionales; los vacíos se omiten en el adapter.
type TokenMaintenanceEvent struct {
	Event  TokenMaintenanceEventType
	Count  int64  // número de entradas eliminadas (solo en cleanup_completed)
	Reason string // descripción del error (solo en cleanup_failed)
}

// TokenMaintenanceEventLogger es el puerto para emitir eventos de mantenimiento de tokens.
// El código de aplicación/infraestructura depende de esta interfaz;
// los adapters (stdout JSON canónico, etc.) la implementan.
type TokenMaintenanceEventLogger interface {
	Log(event TokenMaintenanceEvent)
}
