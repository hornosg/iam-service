package adapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"iam/src/auth/domain/port"
)

// SQLRoleResolverAdapter implementa port.RoleResolver leyendo directamente la tabla
// `roles`. Lee solo lo que auth necesita para firmar (slug, permissions, is_active),
// sin depender de la entidad del módulo role (aislamiento de tipos — condición A2).
type SQLRoleResolverAdapter struct {
	db *sql.DB
}

func NewSQLRoleResolverAdapter(db *sql.DB) *SQLRoleResolverAdapter {
	return &SQLRoleResolverAdapter{db: db}
}

var ErrRoleNotFound = errors.New("rol no encontrado")

func (a *SQLRoleResolverAdapter) GetRole(ctx context.Context, roleID uuid.UUID) (*port.ResolvedRole, error) {
	const query = `SELECT slug, permissions, is_active FROM roles WHERE id = $1`

	var slug sql.NullString
	var perms pq.StringArray
	var isActive bool

	err := a.db.QueryRowContext(ctx, query, roleID).Scan(&slug, &perms, &isActive)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRoleNotFound
		}
		return nil, fmt.Errorf("error resolviendo rol %s: %w", roleID, err)
	}

	return &port.ResolvedRole{
		Slug:        slug.String, // "" si el rol legacy aún no tiene slug
		Permissions: []string(perms),
		IsActive:    isActive,
	}, nil
}
