package adapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	sharedservice "github.com/hornosg/go-shared/domain/service"
)

// PostgresTenantResolver implementa port.TenantByUserResolver con un SELECT
// acotado sobre `users`.
//
// ⚠️ El db que se le inyecta es el pool de LOGIN (iam_login), no el de app:
//   * La policy `users_login_lookup` (019) es FOR SELECT y role-gated
//     (`current_user = 'iam_login'`): permite leer la fila sin GUC de tenant.
//     Bajo account_app la policy `tenant_isolation` exigiría app.tenant_id y
//     el lookup devolvería 0 filas (fail-closed) — justo lo que no sirve acá,
//     porque el resolver corre justamente cuando el token no trae tenant.
//   * El grant de 017 es por columnas y cubre tenant_id. La query no toca
//     ninguna columna fuera del grant.
//
// No existe usuario con tenant NULL (users.tenant_id es NOT NULL, 003), así
// que el único "no encontrado" es que el user_id no exista: se devuelve el
// sentinela compartido ErrUserNotFound para que el gate distinga 401 (usuario
// inexistente) de 500 (no se pudo verificar).
type PostgresTenantResolver struct {
	db *sql.DB
}

func NewPostgresTenantResolver(db *sql.DB) *PostgresTenantResolver {
	return &PostgresTenantResolver{db: db}
}

func (a *PostgresTenantResolver) ResolveTenantByUserID(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	const query = `SELECT tenant_id FROM users WHERE id = $1`

	var tenantID uuid.UUID
	if err := a.db.QueryRowContext(ctx, query, userID).Scan(&tenantID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, sharedservice.ErrUserNotFound
		}
		return uuid.Nil, fmt.Errorf("error resolviendo tenant del usuario %s: %w", userID, err)
	}
	return tenantID, nil
}