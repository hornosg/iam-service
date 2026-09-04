package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// TestMockIsTokenRevoked_SemanticaDelCorte fija la fidelidad del mock al SQL
// real del gate (ACC-E02 T8m, nota 1 del gate L4 de T8i).
//
// El SQL evalúa `revoked_at > to_timestamp(iat)`: revoked_at vive con
// precisión de microsegundos (DEFAULT NOW()) y to_timestamp(iat) trunca al
// segundo → un token emitido en el segundo EXACTO del corte queda REVOCADO.
// El mock viejo (`issuedAt < cutAt.Unix()`) lo dejaba pasar: un mock más
// permisivo que producción deja aprobar en unit lo que el HTTP rechaza.
//
// El caso "segundo exacto" tiene espejo HTTP en
// TestRLS_RevokeAllCortePorUsuario (test/rls): mismo veredicto en ambos.
func TestMockIsTokenRevoked_SemanticaDelCorte(t *testing.T) {
	userID := uuid.New()

	// Corte con sub-segundo, como NOW() de Postgres (precisión de µs).
	cut := time.Date(2026, 9, 3, 12, 0, 0, 500_000_000, time.UTC)
	// Corte en el borde exacto del segundo (revoked_at == to_timestamp(iat)):
	// el `>` estricto del SQL NO revoca. Caso límite, por fidelidad.
	cutBorde := time.Date(2026, 9, 3, 12, 0, 5, 0, time.UTC)

	casos := []struct {
		nombre   string
		cut      time.Time
		hayCorte bool
		issuedAt int64
		revocado bool
	}{
		{"emitido un segundo antes del corte", cut, true, cut.Unix() - 1, true},
		{"emitido en el segundo EXACTO del corte (corte con sub-segundo)", cut, true, cut.Unix(), true},
		{"emitido un segundo después del corte", cut, true, cut.Unix() + 1, false},
		{"legacy sin iat (issuedAt=0): cualquier marca viva revoca (fail-closed)", cut, true, 0, true},
		{"corte en el borde exacto del segundo: el > estricto NO revoca", cutBorde, true, cutBorde.Unix(), false},
		{"sin marca de corte no revoca", time.Time{}, false, time.Now().Unix(), false},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			mock := NewMockAuthRepository()
			if c.hayCorte {
				mock.mu.Lock()
				mock.userCuts[userID] = c.cut
				mock.mu.Unlock()
			}

			revocado, err := mock.IsTokenRevoked(context.Background(), uuid.New(), userID, c.issuedAt)
			assert.NoError(t, err)
			assert.Equal(t, c.revocado, revocado,
				"el mock debe replicar `revoked_at > to_timestamp(iat)` del SQL del gate")
		})
	}
}

// TestMockIsTokenRevoked_CasoNegativo_PredicadoPermisible es la forma
// EJECUTABLE y durable de la prueba negativa del contrato de T8m (fase TEST,
// ACC-E02): "con el mock viejo (permisivo) ese test unitario FALLA".
//
// La prueba negativa original se demostró re-inyectando a mano el predicado
// viejo y revirtiéndolo — evidencia de un solo uso. Este test la fija como
// oráculo de regresión: evalúa en línea el predicado permisivo viejo
// (`issuedAt < cutAt.Unix()`) en el segundo exacto del corte, exige que el
// viejo deje pasar el token (veredicto false) y que el mock actual lo revoque
// (veredicto true). Si alguien vuelve a subir un mock más laxo que el SQL,
// ambos veredictos convergen y el assert de divergencia FALLA — el test
// degrada al comportamiento que el contrato prohíbe, sin necesidad de
// re-inyectar nada a mano.
func TestMockIsTokenRevoked_CasoNegativo_PredicadoPermisible(t *testing.T) {
	userID := uuid.New()

	// Corte con sub-segundo (NOW() de Postgres, precisión de µs). Es la única
	// configuración donde el predicado viejo y el SQL divergen: con el corte
	// en el borde exacto del segundo ambos dan "no revocado" (el `>` estricto).
	cut := time.Date(2026, 9, 3, 12, 0, 0, 500_000_000, time.UTC)
	issuedAt := cut.Unix() // token emitido en el segundo EXACTO del corte

	// Predicado permisivo viejo (pre-T8m): trunca el corte al segundo.
	predicadoViejo := issuedAt < cut.Unix()

	// El viejo dejaba pasar en unit lo que el SQL de producción revoca.
	assert.False(t, predicadoViejo,
		"precondición: el predicado viejo debe dejar pasar el segundo exacto — si esto falla, el caso de divergencia dejó de existir y el contrato de T8m hay que re-evaluarlo")

	mock := NewMockAuthRepository()
	mock.mu.Lock()
	mock.userCuts[userID] = cut
	mock.mu.Unlock()

	revocado, err := mock.IsTokenRevoked(context.Background(), uuid.New(), userID, issuedAt)
	assert.NoError(t, err)
	assert.True(t, revocado,
		"el mock debe revocar el segundo exacto del corte, como el SQL")
	assert.NotEqual(t, predicadoViejo, revocado,
		"REGRESIÓN T8m: el mock volvió a ser tan permisivo como el predicado viejo — está dejando pasar en unit lo que producción (revoked_at > to_timestamp(iat)) rechaza")
}
