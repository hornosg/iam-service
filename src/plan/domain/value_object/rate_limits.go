package value_object

// RateLimitRule es la política de rate limiting de una feature dentro de un plan
// (ADR-003 D4/D5). El algoritmo se elige por celda (plan × feature):
//   - "sliding_window_counter": usa Limit + Window (preciso, rechaza al exceder).
//   - "gcra": usa Rate (ej. "10/s") + Burst (ritmo constante, sin cola real).
// Los servicios consumidores resuelven tier → estas reglas contra una matriz cacheada.
type RateLimitRule struct {
	Algorithm string `json:"algorithm"`        // sliding_window_counter | gcra
	Limit     int    `json:"limit,omitempty"`  // sliding_window_counter
	Window    string `json:"window,omitempty"` // sliding_window_counter (ej. "1s", "1m", "1h")
	Rate      string `json:"rate,omitempty"`   // gcra (ej. "10/s")
	Burst     int    `json:"burst,omitempty"`  // gcra
}

// RateLimits mapea feature/usecase (ej. "ai.bi_query") → su regla en este plan.
// Se persiste como JSONB en la columna plans.rate_limits.
type RateLimits map[string]RateLimitRule
