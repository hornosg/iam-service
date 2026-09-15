package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hornosg/go-shared/criteria"

	"github.com/hornosg/iam-service/src/identity/domain/exception"
	"github.com/hornosg/iam-service/src/identity/domain/port"
	"github.com/hornosg/iam-service/src/identity/domain/value_object"
	"github.com/hornosg/iam-service/src/identity/infrastructure/persistence/repository"
	sharedctx "github.com/hornosg/iam-service/src/shared/context"
	"github.com/hornosg/iam-service/test/testsupport/fakedb"
)

// ACC-E08 T2 (fase TEST): completa los caminos de RECHAZO del contrato de
// PostgresUserRepository que la implementación de la tarea no cubrió. Cada
// test es negativo en el sentido del criterio: afirma qué DEBE fallar cuando
// el control correspondiente no está — un ErrNoRows sin traducción al
// sentinela de dominio, un criterio con campo inyectable que llegara a la
// base, o un error de iteración que devolviera una lista parcial como éxito.

func ctxConTenant() context.Context {
	return sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantIDDePrueba))
}

// criteriaInvalido arma un criteria cuyo nombre de campo no pasa
// ValidateFieldName ([a-zA-Z_][a-zA-Z0-9_.]*) — el conversor debe rechazarlo
// antes de interpolarlo, y el repositorio no debe emitir ni un query.
func criteriaInvalido() criteria.Criteria {
	return criteria.NewCriteria(
		criteria.NewFilters(criteria.NewFilter("status; DROP TABLE users", criteria.OpEqual, "active")),
		nil,
		criteria.NewPagination(1, 10),
	)
}

func TestGetByEmail_SinFilasDevuelveNoEncontrado(t *testing.T) {
	// El login es el único consumidor de GetByEmail (pre-auth): un
	// sql.ErrNoRows crudo se convertiría en 500 en vez de credencial inválida.
	// NEGATIVA: si la traducción al sentinela desaparece, este test falla.
	db, _ := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)

	_, err := repo.GetByEmail(context.Background(), "nadie@example.com", nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, exception.ErrUserNotFound,
		"un email inexistente en el login NO es un error interno: es ErrUserNotFound")
}

func TestGetByEmail_EmailCorruptoEnLaFilaFalla(t *testing.T) {
	// scanCredentialUser (el path de 8 columnas de la migración 017) valida los
	// value objects igual que la fila completa: un dato inválido en la fila no
	// se hidrata a la fuerza.
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	fila := filaCredencial()
	fila[1] = "no-es-un-email"
	conn.EnqueueResult(&fakedb.ResultSet{Rows: [][]driverValue{fila}})

	_, err := repo.GetByEmail(context.Background(), "user@example.com", nil)

	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid email in database")
}

func TestGetByEmail_ErrorDeMotorNoEsNoEncontrado(t *testing.T) {
	// Un fallo del motor en el path de credencial no puede disfrazarse de
	// "usuario inexistente": escondería un 500 detrás de un 401 y bloquearía
	// a un usuario real sin rastro del error real.
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	conn.ExecErr = errors.New("conexión rota")
	conn.FailQueryContains = "FROM users"

	_, err := repo.GetByEmail(context.Background(), "user@example.com", nil)

	require.Error(t, err)
	assert.NotErrorIs(t, err, exception.ErrUserNotFound,
		"el error del motor debe propagarse como error interno, no como usuario inexistente")
}

func TestUpdate_ErrorDeMotorSeEnvuelve(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	conn.ExecErr = errors.New("conexión rota")
	conn.FailQueryContains = "UPDATE users"

	err := repo.Update(ctxConTenant(), userMother())

	require.Error(t, err)
	assert.ErrorContains(t, err, "error updating user")
	assert.NotErrorIs(t, err, exception.ErrUserNotFound,
		"un fallo del motor no es 'usuario no encontrado'")
}

func TestDelete_ErrorDeMotorSeEnvuelve(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	conn.ExecErr = errors.New("conexión rota")
	conn.FailQueryContains = "UPDATE users"

	err := repo.Delete(ctxConTenant(), uuid.New())

	require.Error(t, err)
	assert.ErrorContains(t, err, "error deleting user")
	assert.NotErrorIs(t, err, exception.ErrUserNotFound)
}

func TestListados_ErrorDeMotorSeEnvuelve(t *testing.T) {
	casos := []struct {
		nombre   string
		ejecutar func(repo port.UserCriteriaRepository, ctx context.Context) error
		consulta string
	}{
		{"GetByTenant", func(r port.UserCriteriaRepository, ctx context.Context) error {
			_, err := r.GetByTenant(ctx, uuid.MustParse(tenantIDDePrueba), 10, 0)
			return err
		}, "FROM users"},
		{"GetByStatus", func(r port.UserCriteriaRepository, ctx context.Context) error {
			_, err := r.GetByStatus(ctx, value_object.StatusActive, 10, 0)
			return err
		}, "FROM users"},
		{"GetByRole", func(r port.UserCriteriaRepository, ctx context.Context) error {
			_, err := r.GetByRole(ctx, uuid.MustParse(roleIDDePrueba), 10, 0)
			return err
		}, "FROM users"},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			db, conn := fakedb.New(t)
			repo := repository.NewPostgresUserRepository(db)
			conn.ExecErr = errors.New("conexión rota")
			conn.FailQueryContains = c.consulta

			err := c.ejecutar(repo, ctxConTenant())

			require.Error(t, err)
			assert.ErrorContains(t, err, "error querying")
		})
	}
}

func TestExistsByEmail_ErrorDeMotorSeEnvuelve(t *testing.T) {
	// Un EXISTS cuyo escaneo falla NO puede devolver false: un falso "no
	// existe" deja registrar un email duplicado en el registro.
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	conn.ExecErr = errors.New("conexión rota")
	conn.FailQueryContains = "FROM users"

	exists, err := repo.ExistsByEmail(context.Background(), "user@example.com", nil)

	require.Error(t, err)
	assert.False(t, exists)
	assert.ErrorContains(t, err, "error checking if user exists")
}

func TestConteos_ErrorDeMotorSeEnvuelve(t *testing.T) {
	t.Run("CountByTenant", func(t *testing.T) {
		db, conn := fakedb.New(t)
		repo := repository.NewPostgresUserRepository(db)
		conn.ExecErr = errors.New("conexión rota")
		conn.FailQueryContains = "COUNT(*)"

		count, err := repo.CountByTenant(ctxConTenant(), uuid.MustParse(tenantIDDePrueba))

		assert.Equal(t, 0, count)
		require.Error(t, err)
		assert.ErrorContains(t, err, "error counting users by tenant")
	})

	t.Run("CountByStatus", func(t *testing.T) {
		db, conn := fakedb.New(t)
		repo := repository.NewPostgresUserRepository(db)
		conn.ExecErr = errors.New("conexión rota")
		conn.FailQueryContains = "COUNT(*)"

		count, err := repo.CountByStatus(ctxConTenant(), value_object.StatusActive)

		assert.Equal(t, 0, count)
		require.Error(t, err)
		assert.ErrorContains(t, err, "error counting users by status")
	})
}

func TestSearchByCriteria_CampoInvalidoNoLlegaALaBase(t *testing.T) {
	// El conversor valida el nombre de campo antes de interpolar (defensa
	// contra SQL injection). NEGATIVA: si el campo inyectable llega a la
	// consulta, o si el error no se distingue de un error de motor, este test
	// falla — un campo inválido NO debe emitir ni un solo query a la base.
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	crit := criteriaInvalido()

	_, err := repo.SearchByCriteria(context.Background(), crit)

	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid criteria")
	for _, stmt := range conn.Statements() {
		assert.NotContains(t, stmt, "FROM users",
			"un criteria con campo inyectable no debe llegar a la base")
	}
}

func TestCountByCriteria_CampoInvalidoNoLlegaALaBase(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)

	_, err := repo.CountByCriteria(context.Background(), criteriaInvalido())

	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid criteria")
	for _, stmt := range conn.Statements() {
		assert.NotContains(t, stmt, "COUNT(*)")
	}
}

func TestSearchByCriteria_ErrorDeMotorSeEnvuelve(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	conn.ExecErr = errors.New("conexión rota")
	conn.FailQueryContains = "FROM users"

	crit := criteria.NewCriteria(
		criteria.NewFilters(criteria.NewFilter("status", criteria.OpEqual, value_object.StatusActive.String())),
		nil,
		criteria.NewPagination(1, 10),
	)

	_, err := repo.SearchByCriteria(context.Background(), crit)

	require.Error(t, err)
	assert.ErrorContains(t, err, "error executing search query")
}

func TestCountByCriteria_ErrorDeMotorSeEnvuelve(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	conn.ExecErr = errors.New("conexión rota")
	conn.FailQueryContains = "COUNT(*)"

	crit := criteria.NewCriteria(
		criteria.NewFilters(criteria.NewFilter("tenant_id", criteria.OpEqual, tenantIDDePrueba)),
		nil,
		criteria.NewPagination(1, 10),
	)

	_, err := repo.CountByCriteria(context.Background(), crit)

	require.Error(t, err)
	assert.ErrorContains(t, err, "error executing count query")
}

func TestScanUsers_ErrorDeIteracionNoDevuelveListaParcial(t *testing.T) {
	// La conexión se corta DESPUÉS de la primera fila: devolver la fila leída
	// como resultado válido sería una lista silenciosamente incompleta (el
	// admin vería la mitad de sus usuarios y jamás lo sabría).
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	conn.EnqueueResult(&fakedb.ResultSet{
		Rows:    [][]driverValue{filaCompleta()},
		IterErr: errors.New("conexión cortada a mitad de la iteración"),
	})

	users, err := repo.GetByTenant(ctxConTenant(), uuid.MustParse(tenantIDDePrueba), 10, 0)

	require.Error(t, err)
	assert.ErrorContains(t, err, "error iterating user rows")
	assert.Empty(t, users, "un fallo de iteración no devuelve una lista parcial como éxito")
}