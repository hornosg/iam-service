package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/hornosg/go-shared/infrastructure/env"
	"github.com/hornosg/go-shared/infrastructure/postgres"

	sharedpostgres "iam/src/shared/postgres"
	"iam/src/shared/validator"

	sharedmigrate "github.com/hornosg/go-shared/migrate"

	iamroot "iam"
)

func init() {
	validator.RegisterCustomValidators()
}

func main() {
	// Configuración de la base de datos: dos pools según ACC-E02 T2/T5.
	//   * appDB: account_app — rol de aplicación con RLS (todo caso de uso
	//     con tenant conocido y operaciones post-auth del login).
	//   * loginDB: iam_login — rol acotado de pre-auth; sólo resuelve
	//     credenciales sin filtro de tenant (T1-D1, T1-D2).
	appDB, loginDB, err := setupDatabases()
	if err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}
	defer appDB.Close()
	defer loginDB.Close()

	// Fail-fast anti-superuser (ACC-E02 T6, patrón PLAT-E29 T7 / RULE-09/RULE-10): el runtime
	// NUNCA debe correr como superuser/BYPASSRLS. FORCE ROW LEVEL SECURITY no aplica a superusers →
	// con un rol privilegiado la RLS de users, tenants, refresh_tokens y revoked_tokens queda inerte:
	// el servicio serviría datos cross-tenant sin error visible. Se verifica en AMBOS pools (T1-D2:
	// dos pools reales, appDB con account_app y loginDB con iam_login; ambos deben ser NOBYPASSRLS)
	// y, desde T8n, también sobre la conexión de migraciones (account_migrator): el conteo de
	// guards es el de conexiones abiertas.
	if err := sharedpostgres.AssertNoRLSBypass(appDB); err != nil {
		log.Fatalf("%v", err)
	}
	if err := sharedpostgres.AssertNoRLSBypass(loginDB); err != nil {
		log.Fatalf("%v", err)
	}

	// Migraciones versionadas in-app (ADR-001) — fail-fast antes de servir tráfico.
	// ACC-E02 T8n: corren con el rol dedicado account_migrator (DDL sobre iam_db, SIN
	// uso en runtime), en una conexión propia que se cierra al terminar de migrar —
	// NO sobre el pool de aplicación: account_app sólo tiene SELECT sobre
	// schema_migrations (rol de menor privilegio, correcto para runtime), así que
	// todo arranque con una migración pendiente fallaba con permission denied y el
	// workaround era el baile de dos arranques con DB_USER=postgres +
	// ALLOW_SUPERUSER_DB=true. Bootstrap del rol: scripts/bootstrap_migrator.sh.
	migrateDB, err := setupMigratorDatabase()
	if err != nil {
		log.Fatalf("Error connecting migration role: %v", err)
	}
	// El guard de T6 no se relaja con el tercer rol: tercera conexión abierta →
	// tercer guard. Un rol SUPERUSER/BYPASSRLS en DB_MIGRATE_USER migraría con la
	// RLS inerte y se rechaza igual que en runtime.
	if err := sharedpostgres.AssertNoRLSBypass(migrateDB); err != nil {
		log.Fatalf("%v", err)
	}
	dbName := env.Get("DB_NAME", "iam_db")
	if err := sharedmigrate.RunMigrations(migrateDB, iamroot.MigrationsFS, dbName); err != nil {
		log.Fatalf("Error running migrations: %v", err)
	}
	// Cerrada al terminar de migrar (decisión del owner, T8n): no es un pool de
	// servicio y jamás corre queries de negocio.
	if err := migrateDB.Close(); err != nil {
		log.Printf("cierre de la conexión de migraciones: %v", err)
	}

	// Configuración del router (ACC-E01 T8: construcción aislada en buildRouter — src/router.go)
	router := buildRouter(appDB, loginDB)

	// Iniciar el servidor
	port := env.Get("PORT", "8080")
	log.Printf("Starting IAM server on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}

func setupDatabases() (appDB *sql.DB, loginDB *sql.DB, err error) {
	// Configuración compartida de la base de datos desde variables de entorno.
	host := env.Get("DB_HOST", "localhost")
	port := env.Get("DB_PORT", "5432")
	// ACC-E02 T5: cada pool tiene su propia credencial. Un único password
	// compartido haría que comprometer iam_login (pre-auth) entregue account_app.
	appPassword := env.Get("DB_PASSWORD", "lab_account_app")
	loginPassword := env.Get("DB_LOGIN_PASSWORD", "lab_iam_login")
	dbname := env.Get("DB_NAME", "iam_db")
	sslmode := env.Get("DB_SSLMODE", "disable")

	// Pool de aplicación: account_app (RLS, todo excepto lookup de credencial).
	appUser := env.Get("DB_USER", "account_app")
	appDB, err = postgres.Connect(postgres.Config{
		Host:     host,
		Port:     port,
		User:     appUser,
		Password: appPassword,
		DBName:   dbname,
		SSLMode:  sslmode,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("connect account_app: %w", err)
	}
	postgres.StartPoolMonitor(context.Background(), appDB, postgres.MonitorOptions{
		Service: "iam-service",
		DBName:  dbname,
	})

	// Pool de login: iam_login (sólo resuelve credenciales pre-auth).
	loginUser := env.Get("DB_LOGIN_USER", "iam_login")
	loginDB, err = postgres.Connect(postgres.Config{
		Host:     host,
		Port:     port,
		User:     loginUser,
		Password: loginPassword,
		DBName:   dbname,
		SSLMode:  sslmode,
	})
	if err != nil {
		_ = appDB.Close()
		return nil, nil, fmt.Errorf("connect iam_login: %w", err)
	}
	postgres.StartPoolMonitor(context.Background(), loginDB, postgres.MonitorOptions{
		Service: "iam-service-login",
		DBName:  dbname,
	})

	log.Printf("Successfully connected to database as app=%s login=%s", appUser, loginUser)
	return appDB, loginDB, nil
}

// setupMigratorDatabase abre la conexión dedicada de migraciones (ACC-E02 T8n): rol
// account_migrator con privilegio de DDL sobre iam_db y SIN uso en runtime. Es una
// conexión, no un pool de servicio: no lleva monitor de métricas (viviría lo que
// dura el boot) y se cierra en main() apenas termina RunMigrations. La credencial
// sale de DB_MIGRATE_USER / DB_MIGRATE_PASSWORD y NO comparte secreto con los otros
// roles — misma decisión de T5: una credencial comprometida no entrega las demás.
func setupMigratorDatabase() (*sql.DB, error) {
	host := env.Get("DB_HOST", "localhost")
	port := env.Get("DB_PORT", "5432")
	migrateUser := env.Get("DB_MIGRATE_USER", "account_migrator")
	migratePassword := env.Get("DB_MIGRATE_PASSWORD", "lab_account_migrator")
	dbname := env.Get("DB_NAME", "iam_db")
	sslmode := env.Get("DB_SSLMODE", "disable")

	db, err := postgres.Connect(postgres.Config{
		Host:     host,
		Port:     port,
		User:     migrateUser,
		Password: migratePassword,
		DBName:   dbname,
		SSLMode:  sslmode,
	})
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", migrateUser, err)
	}

	log.Printf("Successfully connected to database as migrator=%s", migrateUser)
	return db, nil
}

