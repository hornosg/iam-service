package port

import (
	"context"

	"github.com/google/uuid"
)

// TenantByUserResolver resuelve el tenant de un usuario a partir de su user_id
// YA VERIFICADO (firma JWT + parse del gate de revocación).
//
// ACC-E02 T8g, criterio (B) del gate de T8f: un access token sin claim de
// tenant (token legacy emitido antes de que el claim existiera) hoy deja al
// gate sin GUC y a la sesión sin poder cerrarse — logout/revoke-all responden
// 500 sin revocar. Derivar el tenant de la base a partir del user_id firmado
// es el mismo encuadre que el lookup de credencial pre-auth (T2): el tenant no
// se confía al token, se resuelve de la fuente. Corre sobre el pool de login
// (iam_login), cuya policy users_login_lookup (019) le permite el SELECT sin
// filtro de tenant y cuyo grant de columnas (017) incluye tenant_id.
type TenantByUserResolver interface {
	ResolveTenantByUserID(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
}