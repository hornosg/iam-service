package usecase

import (
	"context"

	"github.com/google/uuid"

	"iam/src/auth/domain/port"
)

// resolveRoleClaims resuelve el rol del usuario para poblar los claims `roles`/`perms`
// del JWT. Es FAIL-CLOSED (condición de seguridad O2): ante rol inactivo, rol sin slug,
// rol inexistente o cualquier error de resolución, devuelve slices vacíos — el token se
// emite SIN roles y el enforcement downstream (RequireRole) lo rechazará con 403.
//
// Nunca propaga el error: un fallo al resolver el rol no debe impedir el login, solo
// degrada el token a "sin privilegios".
func resolveRoleClaims(ctx context.Context, resolver port.RoleResolver, roleID uuid.UUID) (roles []string, perms []string) {
	if resolver == nil {
		return nil, nil
	}

	resolved, err := resolver.GetRole(ctx, roleID)
	if err != nil || resolved == nil || !resolved.IsActive || resolved.Slug == "" {
		return nil, nil
	}

	return []string{resolved.Slug}, resolved.Permissions
}
