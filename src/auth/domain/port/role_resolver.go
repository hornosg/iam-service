package port

import (
	"context"

	"github.com/google/uuid"
)

// ResolvedRole es la vista que el módulo auth necesita de un rol para firmarlo en el
// JWT. Es un tipo PROPIO de auth (condición de arquitectura A2): el puerto no importa
// entidades del módulo `role` — el acoplamiento queda confinado al adapter.
type ResolvedRole struct {
	Slug        string
	Permissions []string
	IsActive    bool
}

// RoleResolver resuelve un rol por su ID al momento de emitir el token (login/refresh),
// para poblar los claims `roles`/`perms`. El enforcement downstream es 100% offline:
// se resuelve una vez en la emisión, no en cada request.
type RoleResolver interface {
	// GetRole devuelve la vista de autorización del rol. Si el rol no existe debe
	// devolver error; el caso "rol inactivo" se expresa con IsActive=false (fail-closed
	// lo decide el caller, no el resolver).
	GetRole(ctx context.Context, roleID uuid.UUID) (*ResolvedRole, error)
}
