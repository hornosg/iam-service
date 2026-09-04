package postgres

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/hornosg/go-shared/infrastructure/env"
)

// guardQuery resuelve si el rol actual elude la RLS por atributo propio o por
// membresía. La primer rama es la original de T6 (rolsuper/rolbypassrls). La
// segunda (T8o) mira pg_auth_members: los atributos NO se heredan por membresía,
// pero un miembro puede hacer SET ROLE al rol privilegiado y ganar BYPASSRLS
// dentro de la sesión — la membresía es una escalada de privilegios real, no un
// dato decorativo. El CTE recursivo cubre cadenas (account_app ∈ mid ∈ bypass),
// no sólo el primer nivel: cerrar la clase entera, no el caso puntual.
const guardQuery = `
WITH RECURSIVE memberships(roleid) AS (
    SELECT m.roleid
      FROM pg_auth_members m
     WHERE m.member = (SELECT oid FROM pg_roles WHERE rolname = current_user)
    UNION
    SELECT m2.roleid
      FROM pg_auth_members m2
      JOIN memberships ms ON m2.member = ms.roleid
)
SELECT rolsuper OR rolbypassrls
       OR EXISTS (
           SELECT 1
             FROM memberships ms
             JOIN pg_roles g ON g.oid = ms.roleid
            WHERE g.rolsuper OR g.rolbypassrls
       )
  FROM pg_roles
 WHERE rolname = current_user`

// AssertNoRLSBypass aborta el arranque si el rol de base de datos con el que
// conectamos es superuser, tiene el atributo BYPASSRLS, o es miembro —directa o
// transitivamente— de un rol que cumpla alguna de las dos (ACC-E02 T6, patrón
// PLAT-E29 T7 / RULE-09/RULE-10; T8o agrega las membresías).
//
// Con un rol así, FORCE ROW LEVEL SECURITY no se aplica y la RLS de users,
// tenants, refresh_tokens y revoked_tokens queda inerte: el servicio serviría
// datos cross-tenant sin ningún error visible. Convierte ese fail-OPEN silencioso
// en fail-CLOSED ruidoso. Se corre en TODAS las conexiones que abre el servicio
// (account_app, iam_login y, desde T8n, account_migrator): el conteo de llamadas
// = conexiones abiertas.
//
// ALLOW_SUPERUSER_DB=true es un escape hatch explícito para tareas admin locales.
// Desde T8o (residual 1 del GO de @dev-security sobre el cierre de ACC-E02) ya no
// alcanza con setearlo: exige además un marcador explícito de no-producción
// (ENVIRONMENT ∈ {local, lab, test}). Con cualquier otro valor —o sin ENVIRONMENT—
// el arranque se niega: en producción la escotilla no existe, y la única cosa que
// antes lo separaba era un comentario.
func AssertNoRLSBypass(db *sql.DB) error {
	if env.Get("ALLOW_SUPERUSER_DB", "false") == "true" {
		environment := env.Get("ENVIRONMENT", "")
		if !isNonProductionEnvironment(environment) {
			return fmt.Errorf(
				"negativa a arrancar: ALLOW_SUPERUSER_DB=true exige un marcador explícito de "+
					"no-producción (ENVIRONMENT ∈ {local, lab, test}) y ENVIRONMENT=%q no lo es "+
					"o no está seteado (ACC-E02 T8o). En producción la escotilla no existe: usá un "+
					"rol NOBYPASSRLS como account_app/iam_login/account_migrator",
				environment)
		}
		log.Printf("⚠️  ALLOW_SUPERUSER_DB=true — se omite el chequeo NOBYPASSRLS (escape de admin, ENVIRONMENT=%s; NUNCA en producción)", environment)
		return nil
	}

	var privileged bool
	if err := db.QueryRow(guardQuery).Scan(&privileged); err != nil {
		return fmt.Errorf("no se pudo verificar los privilegios del rol de DB (current_user): %w", err)
	}
	if privileged {
		return fmt.Errorf("negativa a arrancar: el rol de DB actual es SUPERUSER o BYPASSRLS " +
			"—directo o por membresía en pg_auth_members— y eludiría la row-level security de " +
			"users/tenants/refresh_tokens/revoked_tokens (ACC-E02 T6/T8o, RULE-09/RULE-10). " +
			"Usá un rol NOBYPASSRLS como account_app/iam_login, o exportá ALLOW_SUPERUSER_DB=true " +
			"junto a ENVIRONMENT=local|lab|test solo para tareas admin")
	}

	log.Println("RLS guard OK: el rol de DB es NOBYPASSRLS y no es miembro de ningún rol con BYPASSRLS (users, tenants, refresh_tokens y revoked_tokens protegidas por FORCE ROW LEVEL SECURITY)")
	return nil
}

// isNonProductionEnvironment dice si el marcador de entorno habilita la escotilla
// de admin. Un entorno desconocido o ausente NO habilita: la condición es el
// marcador explícito, no la ausencia de uno que diga "production".
func isNonProductionEnvironment(environment string) bool {
	switch environment {
	case "local", "lab", "test":
		return true
	default:
		return false
	}
}