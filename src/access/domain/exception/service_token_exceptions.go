package exception

import "errors"

// Errores de la emisión de tokens de servicio (ACC-E09 T2, ADR-006 «Emisión y renovación»).
// El handler los traduce a HTTP: cada uno tiene un único código y nunca se degradan entre sí.
var (
	// ErrServiceTokenInvalidRequest — parámetro del protocolo mal formado (p.ej. client_assertion_type
	// ajeno, RFC 7523 §3.2). 400 invalid_request.
	ErrServiceTokenInvalidRequest = errors.New("s2s token: parámetro inválido")
	// ErrServiceTokenBadRequest — grant_type fuera del protocolo. 400 unsupported_grant_type.
	ErrServiceTokenBadRequest = errors.New("s2s token: solicitud inválida")
	// ErrServiceTokenUnauthenticated — aserción ausente, malformada, con firma inválida, vencida,
	// de un emisor sin clave registrada o con jti ya visto. 401.
	ErrServiceTokenUnauthenticated = errors.New("s2s token: emisor no autenticado")
	// ErrServiceTokenForbidden — audience o scope fuera del registro del emisor, scopes de
	// usuario y de sistema mezclados, o tenant_scope=system no habilitado. 403.
	ErrServiceTokenForbidden = errors.New("s2s token: fuera del registro del emisor")
	// ErrServiceTokenUnavailable — el almacén anti-replay no responde: la emisión falla cerrada
	// en vez de saltear el chequeo. 503.
	ErrServiceTokenUnavailable = errors.New("s2s token: almacén anti-replay no disponible")
	// ErrServiceTokenRateLimited — el emisor ya verificado agotó su cupo. 429. Se descuenta
	// recién tras verificar la firma: un iss sin verificar nunca es clave de un contador.
	ErrServiceTokenRateLimited = errors.New("s2s token: cupo del emisor agotado")
)
