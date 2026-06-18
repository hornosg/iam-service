package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"iam/src/auth/domain/port"
	"iam/src/auth/domain/value_object"
)

type stubPlanResolver struct {
	plan *port.ResolvedPlan
	err  error
}

func (s stubPlanResolver) GetPlanForTenant(_ context.Context, _ uuid.UUID) (*port.ResolvedPlan, error) {
	return s.plan, s.err
}

func TestResolvePlanClaim(t *testing.T) {
	tid := uuid.New()
	planID := uuid.New()

	cases := []struct {
		name     string
		resolver port.PlanResolver
		wantTier string
		wantPID  uuid.UUID
	}{
		{
			name:     "plan activo → tier real + plan_id",
			resolver: stubPlanResolver{plan: &port.ResolvedPlan{Tier: "PREMIUM", PlanID: planID, IsActive: true}},
			wantTier: "PREMIUM",
			wantPID:  planID,
		},
		{
			name:     "plan inactivo → degrada a FREE",
			resolver: stubPlanResolver{plan: &port.ResolvedPlan{Tier: "PREMIUM", PlanID: planID, IsActive: false}},
			wantTier: value_object.PlanTierFree,
			wantPID:  uuid.Nil,
		},
		{
			name:     "tenant sin plan (error) → degrada a FREE, no propaga",
			resolver: stubPlanResolver{err: errors.New("plan not found")},
			wantTier: value_object.PlanTierFree,
			wantPID:  uuid.Nil,
		},
		{
			name:     "tier vacío → degrada a FREE",
			resolver: stubPlanResolver{plan: &port.ResolvedPlan{Tier: "", IsActive: true}},
			wantTier: value_object.PlanTierFree,
			wantPID:  uuid.Nil,
		},
		{
			name:     "resolver nil → FREE",
			resolver: nil,
			wantTier: value_object.PlanTierFree,
			wantPID:  uuid.Nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claim := resolvePlanClaim(context.Background(), tc.resolver, tid)
			if claim == nil {
				t.Fatal("resolvePlanClaim nunca debe devolver nil para tokens nuevos")
			}
			if claim.Tier != tc.wantTier {
				t.Errorf("tier = %q, want %q", claim.Tier, tc.wantTier)
			}
			if claim.PlanID != tc.wantPID {
				t.Errorf("plan_id = %v, want %v", claim.PlanID, tc.wantPID)
			}
		})
	}
}
