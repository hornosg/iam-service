-- Migration: revoke-all deja de ser decorativo (ACC-E02 T8i)
-- Description: `POST /auth/revoke-all` insertaba en revoked_tokens un JTI
--   aleatorio (uuid.New()) que el gate de revocación jamás consulta —
--   IsTokenRevoked busca el JTI DEL token presentado. Resultado: revoke-all
--   respondía OK habiendo insertado una fila decorativa y TODOS los access
--   tokens del usuario seguían válidos hasta expirar. Quien sospecha que le
--   robaron la sesión y pide cerrar todas, no cerraba ninguna.
--
-- === Decisión de diseño (criterio (b) de T8i) ===
--
-- La intención original está en el esquema a medio implementar:
-- 009 ya define `user_id NOT NULL` y crea `idx_revoked_tokens_user_id` —
-- pero el gate nunca consultaba por user_id ("el índice existe y no lo usa
-- nadie", verificado por el owner en el gate L4 de T8g). Se materializa esa
-- intención con la semántica de CORTE sugerida por la tarea:
--
--   * `RevokeAllUserTokens` inserta una marca de alcance USER
--     (scope='user', user_id, revoked_at=NOW()).
--   * El gate (`IsTokenRevoked`) coteja esa marca contra el `iat` del token:
--     queda revocado todo access token del usuario EMITIDO ANTES del corte
--     (`revoked_at > iat`). Los emitidos después siguen válidos — el usuario
--     puede volver a loguearse sin que la marca lo persiga.
--
-- Por qué un corte por iat y no borrar/invalidar de otra forma: los access
-- tokens son stateless (TTL 15 min, sin estado en servidor); la única forma
-- de invalidarlos antes de tiempo es que el gate de cada request consulte
-- algo que los alcance. El `iat` ya viaja implícito en la semántica del JWT
-- y es lo único que distingue "emitido antes del corte" de "emitido después".
--
-- Por qué la columna `scope` y no deducir el tipo de fila: logout (RevokeToken)
-- inserta marcas por JTI de UNA sesión. Sin discriminador, un predicado por
-- user_id + revoked_at matchearía TAMBIÉN las filas de JTI de ese usuario y un
-- logout de una sola sesión mataría los tokens de las demás sesiones emitidos
-- antes — cambio de semántica del logout que la tarea excluye explícitamente
-- (criterio (c): "logout de una sola sesión sigue funcionando"). El
-- discriminador explícito mantiene cada marca con su semántica exacta.
--
-- Tokens legacy sin `iat` (claim NUEVO y aditivo en TokenClaims): deserializan
-- a 0 y `revoked_at > to_timestamp(0)` es verdadero para cualquier marca
-- viva → FAIL-CLOSED: tras un revoke-all, los tokens legacy del usuario quedan
-- revocados porque no se puede saber cuándo fueron emitidos. Blast radius
-- acotado a ese usuario (la marca es por user_id), que es exactamente quien
-- pidió cerrar sus sesiones.
--
-- === El "Ojo" de la tarea (índice y RLS) ===
--
--   * Índice: el gate pasa a tener un predicado por user_id en CADA request.
--     `idx_revoked_tokens_user_id` (009) cubre `user_id = $`; se agrega además
--     un índice PARCIAL `(user_id, revoked_at) WHERE scope = 'user'` que es el
--     camino óptimo del gate (las filas scope='user' son una fracción mínima).
--     El test de evidencia verifica por EXPLAIN que la consulta del gate no
--     cae a Seq Scan.
--   * RLS: la policy tenant_isolation de 019 (USING `user_id IN (SELECT id
--     FROM users WHERE tenant_id = app.tenant_id)`) aplica igual al predicado
--     nuevo — el gate corre con el tenant de los claims ya verificados (o
--     derivado del user_id, T8g-B), y una marca de usuario es visible sólo
--     desde la sesión de ESE tenant. El corte por user_id no abre lectura
--     cross-tenant: el EXISTS matchea filas del propio usuario o nada.
--
-- Sin policy nueva ni cambio de policies: la 019/021/022 quedan tal cual
-- (artefactos firmados por sus gates). Esta migración no toca policies.
--
-- Idempotente (requisito de la épica): ADD COLUMN IF NOT EXISTS, DROP
-- CONSTRAINT/INDEX IF EXISTS antes de recrear. Re-aplicarla no falla.
-- NO se aplica al iam_db vivo en esta tarea (sigue en v19): la aplicación
-- real es por RunMigrations en el cutover (misma decisión que T8e-2 (iii) y
-- T8h) — aplicarla ahora con el binario viejo en el lab es inocuo (columna
-- con default), pero se respeta la regla de la épica de no pisar el vivo
-- desde una iteración L4 sin sign-off.

ALTER TABLE revoked_tokens ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT 'jti';

ALTER TABLE revoked_tokens DROP CONSTRAINT IF EXISTS revoked_tokens_scope_check;
ALTER TABLE revoked_tokens ADD CONSTRAINT revoked_tokens_scope_check
  CHECK (scope IN ('jti', 'user'));

DROP INDEX IF EXISTS idx_revoked_tokens_user_scope;
CREATE INDEX idx_revoked_tokens_user_scope
  ON revoked_tokens(user_id, revoked_at)
  WHERE scope = 'user';