package adapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"iam/src/auth/domain/port"
)

// SQLPlanResolverAdapter implementa port.PlanResolver leyendo el plan del tenant con un
// JOIN tenants↔plans. Devuelve solo lo que auth necesita para firmar el tier (aislamiento
// de tipos, igual que SQLRoleResolverAdapter), sin depender del módulo plan/tenant.
type SQLPlanResolverAdapter struct {
	db *sql.DB
}

func NewSQLPlanResolverAdapter(db *sql.DB) *SQLPlanResolverAdapter {
	return &SQLPlanResolverAdapter{db: db}
}

var ErrPlanNotFound = errors.New("plan no encontrado para el tenant")

func (a *SQLPlanResolverAdapter) GetPlanForTenant(ctx context.Context, tenantID uuid.UUID) (*port.ResolvedPlan, error) {
	// INNER JOIN: si el tenant no tiene plan_id (NULL) no hay fila → ErrPlanNotFound →
	// el caller degrada a FREE. IsActive surge del status del plan.
	const query = `
		SELECT p.id, p.type, p.status
		FROM tenants t
		JOIN plans p ON t.plan_id = p.id
		WHERE t.id = $1`

	var planID uuid.UUID
	var tier, status string
	err := a.db.QueryRowContext(ctx, query, tenantID).Scan(&planID, &tier, &status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPlanNotFound
		}
		return nil, fmt.Errorf("error resolviendo plan del tenant %s: %w", tenantID, err)
	}

	return &port.ResolvedPlan{
		Tier:     tier,
		PlanID:   planID,
		IsActive: status == "ACTIVE",
	}, nil
}
