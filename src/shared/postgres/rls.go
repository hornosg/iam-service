package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"sort"

	"github.com/google/uuid"
)

// TenantVar es la GUC que las migraciones RLS de ACC-E02 T4 usan para
// aislar filas por tenant. Debe mantenerse en sync con 019_rls_account.up.sql.
const TenantVar = "app.tenant_id"

// RefreshDigestVar es la GUC del escape de presentación de refresh tokens
// (migración 021, ACC-E02 T8e). El repo la fija —con el DIGEST sha256 (hex de 64)
// del token presentado, nunca el token crudo (elección del gate de T8e-1, la
// policy compara sha256() del lado del motor)— sólo dentro de la transacción que resuelve
// la credencial pre-auth de POST /auth/refresh, análogo a como iam_login usa
// users_login_lookup para el login. La policy refresh_token_presentation deja
// ver únicamente la fila cuyo digest coincide.
const RefreshDigestVar = "app.refresh_digest"

// TokenMaintenanceVar es la GUC que habilita el borrado de revocaciones
// EXPIRADAS (migración 021). La fija únicamente la goroutine de limpieza; un
// request que la fijara podría borrar revocaciones vencidas de cualquier
// tenant, lo que es inofensivo (esos JTIs ya no cubren ningún token vivo) pero
// igual queda prohibido por diseño.
const TokenMaintenanceVar = "app.token_maintenance"

// SystemAdminTrue es el valor con el que se fija SystemAdminVar (el literal que
// la policy castea a bool). Debe mantenerse en sync con 019_rls_account.up.sql.
const SystemAdminTrue = "true"

// SystemAdminVar es la GUC que habilita el branch cross-tenant de la policy de
// `tenants` (019, T4). ACC-E02 T8d: el ÚNICO origen válido del valor es la
// decisión de autorización de Authorize — scope S2S `system:admin` resuelto
// contra el registry, o rol `system_admin` verificado contra allowedRoles.
// Nunca un claim crudo del JWT: un `is_system_admin` inyectado en un token
// tenant_admin no abre nada (test de T8d). Sin GUC → NULL → fail-closed.
const SystemAdminVar = "app.is_system_admin"

// safeGUCValue restringe lo que se interpola en SET LOCAL (Postgres no acepta
// bind params en SET). Un uuid parseado es seguro; el regex es defensa en
// profundidad para cualquier otro consumidor.
var safeGUCValue = regexp.MustCompile(`^[a-zA-Z0-9_.\-]{0,128}$`)

// safeGUCName restringe el nombre de la variable de sesión interpolada.
var safeGUCName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.]{0,63}$`)

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
	return WithSessionLocals(ctx, db, map[string]string{TenantVar: tenantID.String()}, fn)
}

// WithSessionLocals abre una transacción, fija cada GUC de la lista con SET
// LOCAL y ejecuta fn dentro de ella. Es la forma general de la que
// WithRLSInTransaction es el caso tenant. Las claves del map se aplican en
// orden determinista (sorted) para que los tests puedan afirmar sobre el orden
// de las sentencias.
func WithSessionLocals(ctx context.Context, db *sql.DB, locals map[string]string, fn func(context.Context, *sql.Tx) error) error {
	if len(locals) == 0 {
		return fmt.Errorf("rls: se requiere al menos una variable de sesión")
	}
	keys := make([]string, 0, len(locals))
	for k := range locals {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		if !safeGUCName.MatchString(k) {
			return fmt.Errorf("rls: nombre de GUC inválido: %q", k)
		}
		if !safeGUCValue.MatchString(locals[k]) {
			return fmt.Errorf("rls: valor de GUC inválido para %s", k)
		}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("rls: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, k := range keys {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("SET LOCAL %s = %s", k, quoteLiteral(locals[k]))); err != nil {
			return fmt.Errorf("rls: set %s: %w", k, err)
		}
	}

	if err := fn(ctx, tx); err != nil {
		return err
	}

	return tx.Commit()
}
