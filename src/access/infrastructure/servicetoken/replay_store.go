package servicetoken

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/hornosg/iam-service/src/access/domain/port"
)

// PostgresReplayStore guarda los jti de aserción en s2s_assertion_jti (migración 027). PostgreSQL y
// no Redis: iam ya depende de él y sirve con varias réplicas (ADR-006). Cualquier error se devuelve
// tal cual y el caso de uso falla cerrado (503).
type PostgresReplayStore struct {
	db *sql.DB
}

var _ port.AssertionReplayStore = (*PostgresReplayStore)(nil)

func NewPostgresReplayStore(db *sql.DB) *PostgresReplayStore {
	return &PostgresReplayStore{db: db}
}

func (s *PostgresReplayStore) Remember(ctx context.Context, jti string, until time.Time) (bool, error) {
	if s == nil || s.db == nil {
		return false, fmt.Errorf("s2s replay store: sin base")
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO s2s_assertion_jti (jti, expires_at) VALUES ($1, $2) ON CONFLICT (jti) DO NOTHING`,
		jti, until.UTC())
	if err != nil {
		return false, fmt.Errorf("s2s replay store: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("s2s replay store: %w", err)
	}
	return n == 1, nil
}

// Purge borra los jti vencidos. Un jti vencido ya no puede reutilizarse: su aserción está fuera de
// exp + leeway y el verificador la rechaza antes de llegar acá.
func (s *PostgresReplayStore) Purge(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM s2s_assertion_jti WHERE expires_at < $1`, now.UTC())
	if err != nil {
		return 0, fmt.Errorf("s2s replay store: purga: %w", err)
	}
	return res.RowsAffected()
}
