package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"

	"iam/src/auth/domain/entity"
	"iam/src/auth/domain/port"
	"iam/src/auth/domain/value_object"
	sharedctx "iam/src/shared/context"
	sharedpostgres "iam/src/shared/postgres"
)

type PostgresAuthRepository struct {
	db *sql.DB
}

func NewPostgresAuthRepository(db *sql.DB) port.AuthRepository {
	return &PostgresAuthRepository{
		db: db,
	}
}

type execer interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

// withTenantRLS envuelve fn en una transacción con SET LOCAL app.tenant_id
// tomado del contexto. ACC-E02 T8e: es OBLIGATORIO para toda operación de
// sesión (refresh/logout/revoke/revocación) bajo account_app — las policies de
// refresh_tokens/revoked_tokens no dejan ver ni escribir nada sin app.tenant_id,
// y correr sin él falla cerrado (antes lo hacía en silencio: DELETE/INSERT de 0
// filas sin error). Si el contexto no trae tenant, se rechaza acá, explícito,
// antes de llegar al motor.
func (r *PostgresAuthRepository) withTenantRLS(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	tenantID, ok := sharedctx.TenantIDFromContext(ctx)
	if !ok {
		return fmt.Errorf("auth repo: la operación requiere tenant en el contexto (RLS fail-closed de ACC-E02)")
	}
	return sharedpostgres.WithRLSInTransaction(ctx, r.db, tenantID, fn)
}

// CreateRefreshToken almacena un nuevo refresh token. La inserción corre bajo
// account_app con SET LOCAL app.tenant_id para que la policy RLS de
// refresh_tokens permita la fila (ACC-E02 T4/T5); la columna tenant_id
// denormalizada (021, T8e) toma el tenant de sesión — el mismo que la WITH
// CHECK valida, de modo que el valor escrito y el aislado son uno solo.
func (r *PostgresAuthRepository) CreateRefreshToken(ctx context.Context, token *entity.RefreshToken) error {
	tenantID, ok := sharedctx.TenantIDFromContext(ctx)
	if !ok {
		// T8e: sin tenant en el contexto no hay INSERT que pudiera pasar el WITH
		// CHECK (tenant_id NOT NULL desde la 021). Fallar acá, explícito, en vez
		// de dejar que el motor lo rechace con un error críptico de constraint.
		return fmt.Errorf("auth repo: la creación de refresh token requiere tenant en el contexto (RLS fail-closed de ACC-E02)")
	}
	return sharedpostgres.WithRLSInTransaction(ctx, r.db, tenantID, func(ctx context.Context, tx *sql.Tx) error {
		return r.createRefreshToken(ctx, tx, tenantID, token)
	})
}

func (r *PostgresAuthRepository) createRefreshToken(ctx context.Context, db execer, tenantID uuid.UUID, token *entity.RefreshToken) error {
	query := `
		INSERT INTO refresh_tokens (id, user_id, tenant_id, token, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := db.ExecContext(ctx, query,
		token.ID,
		token.UserID,
		tenantID,
		token.Token,
		token.ExpiresAt,
		token.CreatedAt,
	)

	if err != nil {
		return fmt.Errorf("error creando refresh token: %w", err)
	}

	return nil
}

// GetRefreshToken resuelve el refresh token PRESENTADO. Es el único paso
// pre-auth del refresh: no hay tenant conocido hasta leer la fila, así que no
// corre con app.tenant_id (la policy de tenant lo denegaría para todos) sino
// con el escape de presentación de la migración 021 — GUC app.refresh_digest =
// sha256(token presentado) en hex, dentro de una transacción desechable, que deja ver
// únicamente la fila cuyo digest coincide. Análogo a users_login_lookup para
// el login (T2). La fila trae tenant_id denormalizado: con él, el caso de uso
// fija app.tenant_id y todo lo posterior corre bajo la RLS del tenant dueño.
func (r *PostgresAuthRepository) GetRefreshToken(ctx context.Context, token string) (*entity.RefreshToken, error) {
	var found *entity.RefreshToken
	err := sharedpostgres.WithSessionLocals(ctx, r.db,
		map[string]string{sharedpostgres.RefreshDigestVar: refreshDigest(token)},
		func(ctx context.Context, tx *sql.Tx) error {
			query := `
				SELECT id, user_id, tenant_id, token, expires_at, created_at
				FROM refresh_tokens
				WHERE token = $1`

			rt := &entity.RefreshToken{}
			if err := tx.QueryRowContext(ctx, query, token).Scan(
				&rt.ID,
				&rt.UserID,
				&rt.TenantID,
				&rt.Token,
				&rt.ExpiresAt,
				&rt.CreatedAt,
			); err != nil {
				return err
			}
			found = rt
			return nil
		})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("refresh token no encontrado")
		}
		return nil, fmt.Errorf("error obteniendo refresh token: %w", err)
	}

	return found, nil
}

// refreshDigest es lo único de la credencial presentada que se interpola en el
// GUC de sesión: el digest sha256 del token en hex (64 chars). El token crudo
// no viaja por SET LOCAL (no queda expuesto en pg_stat_activity/pg_stat_statements)
// y quien sólo vea el digest no puede usarlo para presentar el token. El gate de
// T8e-1 fijó sha256 — la policy de la 021 compara sha256() del lado del motor, así
// que md5 acá haría que el escape nunca matchee y POST /auth/refresh dé 401.
func refreshDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// DeleteRefreshToken elimina un refresh token específico EXIGIENDO que la
// eliminación haya borrado exactamente una fila (T8f). La rotación single-use
// es check-then-act en transacciones separadas (Get → Delete → Create) y un
// DELETE que descarta RowsAffected permitía que dos refresh concurrentes con
// el mismo token pasaran ambos el escape de presentación: uno borraba 1, el
// otro 0 sin error, y ambos emitían credenciales nuevas. Bajo READ COMMITTED
// el segundo DELETE se bloquea en el row lock del primero, re-evalúa el WHERE,
// ve 0 filas y aborta acá con ErrRefreshTokenAlreadyConsumed: el consume queda
// atómico. RLS-sensitivo (T8e): sin app.tenant_id la policy lo degradaría a
// DELETE de 0 filas — antes en silencio, ahora también error.
func (r *PostgresAuthRepository) DeleteRefreshToken(ctx context.Context, token string) error {
	return r.withTenantRLS(ctx, func(ctx context.Context, tx *sql.Tx) error {
		query := `DELETE FROM refresh_tokens WHERE token = $1`

		result, err := tx.ExecContext(ctx, query, token)
		if err != nil {
			return fmt.Errorf("error eliminando refresh token: %w", err)
		}

		deleted, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("error obteniendo filas eliminadas del refresh token: %w", err)
		}
		if deleted != 1 {
			return fmt.Errorf("%w: el DELETE tocó %d filas (esperadas 1) — la rotación single-use ya consumió la credencial", port.ErrRefreshTokenAlreadyConsumed, deleted)
		}

		return nil
	})
}

// DeleteAllUserRefreshTokens elimina todos los refresh tokens de un usuario
// (logout). RLS-sensitivo (T8e): requiere tenant en el contexto.
func (r *PostgresAuthRepository) DeleteAllUserRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	return r.withTenantRLS(ctx, func(ctx context.Context, tx *sql.Tx) error {
		query := `DELETE FROM refresh_tokens WHERE user_id = $1`

		_, err := tx.ExecContext(ctx, query, userID)
		if err != nil {
			return fmt.Errorf("error eliminando refresh tokens del usuario: %w", err)
		}

		return nil
	})
}

// RevokeToken inserta un JTI en la tabla de tokens revocados (logout).
// RLS-sensitivo (T8e): el INSERT viola el WITH CHECK sin app.tenant_id.
func (r *PostgresAuthRepository) RevokeToken(ctx context.Context, jti uuid.UUID, userID uuid.UUID, expiresAt time.Time) error {
	return r.withTenantRLS(ctx, func(ctx context.Context, tx *sql.Tx) error {
		query := `
			INSERT INTO revoked_tokens (jti, user_id, expires_at)
			VALUES ($1, $2, $3)
			ON CONFLICT (jti) DO NOTHING`

		_, err := tx.ExecContext(ctx, query, jti, userID, expiresAt)
		if err != nil {
			return fmt.Errorf("error revocando token: %w", err)
		}
		return nil
	})
}

// IsTokenRevoked verifica si el token presentado quedó revocado. Es el gate
// del middleware TokenRevocationCheck: RLS-sensitivo (T8e). Correr sin
// app.tenant_id hacía que la policy devolviera 0 filas SIEMPRE — el gate
// fallaba abierta y un JTI revocado pasaba. El caller (middleware) propaga el
// tenant de los claims ya verificados.
//
// ACC-E02 T8i: además del JTI del token (logout de una sesión), coteja la
// marca de alcance user de revoke-all (migración 023): una fila scope='user'
// del usuario con revoked_at > issuedAt revoca todo token EMITIDO ANTES del
// corte. Antes revoke-all insertaba un JTI aleatorio que esta consulta jamás
// matcheaba: respondía OK sin revocar nada. La marca de usuario NO matchea
// filas de JTI (discriminadas por scope): el logout de una sesión sigue
// revocando sólo esa sesión (criterio (c) de T8i). Un token legacy sin iat
// (issuedAt=0) queda revocado por cualquier marca viva — fail-closed.
func (r *PostgresAuthRepository) IsTokenRevoked(ctx context.Context, jti uuid.UUID, userID uuid.UUID, issuedAt int64) (bool, error) {
	var exists bool
	err := r.withTenantRLS(ctx, func(ctx context.Context, tx *sql.Tx) error {
		query := `SELECT EXISTS(
			SELECT 1 FROM revoked_tokens
			WHERE jti = $1
			   OR (scope = 'user' AND user_id = $2 AND revoked_at > to_timestamp($3))
		)`

		return tx.QueryRowContext(ctx, query, jti, userID, issuedAt).Scan(&exists)
	})
	if err != nil {
		return false, fmt.Errorf("error verificando token revocado: %w", err)
	}
	return exists, nil
}

// RevokeAllUserTokens inserta una marca de alcance user (scope='user',
// migración 023) que el gate coteja contra el iat de cada token: queda
// revocado todo access token del usuario emitido antes del corte (ACC-E02
// T8i). Antes insertaba un JTI aleatorio que IsTokenRevoked jamás consultaba
// — revoke-all respondía OK sin revocar nada. El jti de la fila es un filler
// de la PK (la marca se matchea por user_id + scope, nunca por jti).
// RLS-sensitivo (T8e).
func (r *PostgresAuthRepository) RevokeAllUserTokens(ctx context.Context, userID uuid.UUID, expiresAt time.Time) error {
	jti := uuid.New()
	return r.withTenantRLS(ctx, func(ctx context.Context, tx *sql.Tx) error {
		query := `
			INSERT INTO revoked_tokens (jti, user_id, expires_at, scope)
			VALUES ($1, $2, $3, 'user')`

		_, err := tx.ExecContext(ctx, query, jti, userID, expiresAt)
		if err != nil {
			return fmt.Errorf("error revocando todos los tokens del usuario: %w", err)
		}
		return nil
	})
}

// CleanupExpiredRevocations elimina entradas de revocación expiradas. Corre en
// una goroutine con contexto SIN tenant: la policy revocation_maintenance
// (migración 021) habilita el borrado sólo cuando la app fija
// app.token_maintenance = 'on' dentro de esta transacción.
func (r *PostgresAuthRepository) CleanupExpiredRevocations(ctx context.Context) (int64, error) {
	var count int64
	err := sharedpostgres.WithSessionLocals(ctx, r.db,
		map[string]string{sharedpostgres.TokenMaintenanceVar: "on"},
		func(ctx context.Context, tx *sql.Tx) error {
			query := `DELETE FROM revoked_tokens WHERE expires_at < NOW()`

			result, err := tx.ExecContext(ctx, query)
			if err != nil {
				return fmt.Errorf("error limpiando tokens revocados expirados: %w", err)
			}

			count, err = result.RowsAffected()
			if err != nil {
				return fmt.Errorf("error obteniendo filas afectadas: %w", err)
			}
			return nil
		})
	if err != nil {
		return 0, err
	}
	return count, nil
}

// GetUserByFederatedID obtiene un usuario por su ID federado
func (r *PostgresAuthRepository) GetUserByFederatedID(ctx context.Context, provider value_object.AuthProvider, federatedID string, tenantID *uuid.UUID) (port.UserData, error) {
	query := `
		SELECT id, email, password_hash, tenant_id, role_id, status, provider, federated_id
		FROM users
		WHERE provider = $1 AND federated_id = $2`

	args := []interface{}{provider, federatedID}

	if tenantID != nil {
		query += ` AND tenant_id = $3`
		args = append(args, *tenantID)
	}

	var user port.UserData

	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.TenantID,
		&user.RoleID,
		&user.Status,
		&user.Provider,
		&user.FederatedID,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return user, fmt.Errorf("usuario no encontrado")
		}
		return user, fmt.Errorf("error obteniendo usuario por ID federado: %w", err)
	}

	return user, nil
}

// LinkFederatedID vincula un ID federado a un usuario existente. ACC-E02 T5:
// este UPDATE se ejecuta post-auth bajo account_app con RLS, nunca bajo
// iam_login (T1-D1). Si el contexto NO trae tenant_id, el UPDATE corre sin
// SET LOCAL y la policy descarta la fila en el motor: fail-closed, pero en
// silencio — UPDATE de 0 filas sin error al caller (deuda anotada en T5/T7;
// hacerlo fallar explícito corresponde a la reparación del wiring de tokens
// de sesión, misma familia).
func (r *PostgresAuthRepository) LinkFederatedID(ctx context.Context, userID uuid.UUID, provider value_object.AuthProvider, federatedID string) error {
	if tenantID, ok := sharedctx.TenantIDFromContext(ctx); ok {
		return sharedpostgres.WithRLSInTransaction(ctx, r.db, tenantID, func(ctx context.Context, tx *sql.Tx) error {
			return r.linkFederatedID(ctx, tx, userID, provider, federatedID)
		})
	}
	return r.linkFederatedID(ctx, r.db, userID, provider, federatedID)
}

func (r *PostgresAuthRepository) linkFederatedID(ctx context.Context, db execer, userID uuid.UUID, provider value_object.AuthProvider, federatedID string) error {
	query := `
		UPDATE users
		SET provider = $1, federated_id = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3`

	_, err := db.ExecContext(ctx, query, provider, federatedID, userID)
	if err != nil {
		return fmt.Errorf("error vinculando ID federado: %w", err)
	}

	return nil
}