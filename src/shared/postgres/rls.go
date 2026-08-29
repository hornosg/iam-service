package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"

	"github.com/google/uuid"
)

// TenantVar es la GUC que las migraciones RLS de ACC-E02 T4 usan para
// aislar filas por tenant. Debe mantenerse en sync con 019_rls_account.up.sql.
const TenantVar = "app.tenant_id"

// safeGUCValue restringe lo que se interpola en SET LOCAL (Postgres no acepta
// bind params en SET). Un uuid parseado es seguro; el regex es defensa en
// profundidad para cualquier otro consumidor.
var safeGUCValue = regexp.MustCompile(`^[a-zA-Z0-9_.\-]{0,128}$`)

// quoteLiteral escapa comillas simples para uso en SET LOCAL.
func quoteLiteral(s string) string {
	out := make([]byte, 0, len(s)+2)
	out = append(out, '\'')
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			out = append(out, '\'', '\'')
			continue
		}
		out = append(out, s[i])
	}
	out = append(out, '\'')
	return string(out)
}

// WithRLSInTransaction abre una transacción, fija app.tenant_id con SET LOCAL,
// ejecuta fn y hace commit/rollback. SET LOCAL se descarta al cerrar la tx,
// por lo que no contamina conexiones del pool (patrón de go-shared, replicado
// acá porque la versión consumida por iam-service no lo exporta aún).
func WithRLSInTransaction(ctx context.Context, db *sql.DB, tenantID uuid.UUID, fn func(context.Context, *sql.Tx) error) error {
	if tenantID == uuid.Nil {
		return fmt.Errorf("rls: tenant_id requerido")
	}
	idStr := tenantID.String()
	if !safeGUCValue.MatchString(idStr) {
		return fmt.Errorf("rls: tenant_id con formato inválido")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("rls: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, fmt.Sprintf("SET LOCAL %s = %s", TenantVar, quoteLiteral(idStr))); err != nil {
		return fmt.Errorf("rls: set tenant_id: %w", err)
	}

	if err := fn(ctx, tx); err != nil {
		return err
	}

	return tx.Commit()
}
