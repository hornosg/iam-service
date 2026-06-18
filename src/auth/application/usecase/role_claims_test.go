package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"iam/src/auth/domain/port"
)

type stubRoleResolver struct {
	role *port.ResolvedRole
	err  error
}

func (s stubRoleResolver) GetRole(_ context.Context, _ uuid.UUID) (*port.ResolvedRole, error) {
	return s.role, s.err
}

func TestResolveRoleClaims(t *testing.T) {
	id := uuid.New()

	cases := []struct {
		name      string
		resolver  port.RoleResolver
		wantRoles []string
		wantPerms []string
	}{
		{
			name:      "rol activo con slug → roles y perms poblados",
			resolver:  stubRoleResolver{role: &port.ResolvedRole{Slug: "cashier", Permissions: []string{"sales:pos:sell"}, IsActive: true}},
			wantRoles: []string{"cashier"},
			wantPerms: []string{"sales:pos:sell"},
		},
		{
			name:      "rol inactivo → fail-closed (sin roles)",
			resolver:  stubRoleResolver{role: &port.ResolvedRole{Slug: "cashier", Permissions: []string{"sales:pos:sell"}, IsActive: false}},
			wantRoles: nil,
			wantPerms: nil,
		},
		{
			name:      "rol sin slug (legacy) → fail-closed",
			resolver:  stubRoleResolver{role: &port.ResolvedRole{Slug: "", Permissions: []string{"x"}, IsActive: true}},
			wantRoles: nil,
			wantPerms: nil,
		},
		{
			name:      "error de resolución → fail-closed, no propaga",
			resolver:  stubRoleResolver{err: errors.New("db down")},
			wantRoles: nil,
			wantPerms: nil,
		},
		{
			name:      "resolver nil → fail-closed",
			resolver:  nil,
			wantRoles: nil,
			wantPerms: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			roles, perms := resolveRoleClaims(context.Background(), tc.resolver, id)
			if !equalStrings(roles, tc.wantRoles) {
				t.Errorf("roles = %v, want %v", roles, tc.wantRoles)
			}
			if !equalStrings(perms, tc.wantPerms) {
				t.Errorf("perms = %v, want %v", perms, tc.wantPerms)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
