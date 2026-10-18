package servicetoken

import (
	"sync"
	"time"
)

// WindowLimiter es un contador de ventana fija en memoria (ADR-006 «Rate limit pre-auth»). Con
// una sola réplica de iam alcanza; con varias se revisa junto con el almacén de jti (PLAT-E14).
type WindowLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	now     func() time.Time
	start   time.Time
	counter map[string]int
}

func NewWindowLimiter(limit int, window time.Duration, now func() time.Time) *WindowLimiter {
	if now == nil {
		now = time.Now
	}
	return &WindowLimiter{limit: limit, window: window, now: now, counter: map[string]int{}}
}

// Allow descuenta una unidad del cupo de key en la ventana vigente.
func (l *WindowLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if t := l.now(); t.Sub(l.start) >= l.window {
		l.start = t
		l.counter = map[string]int{}
	}
	if l.counter[key] >= l.limit {
		return false
	}
	l.counter[key]++
	return true
}
