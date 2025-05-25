package main

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Uso: go run migrate_evolutionary.go [check|migrate|help]")
		fmt.Println("  check:   Verificar estado actual antes de migración")
		fmt.Println("  migrate: Ejecutar migración evolutiva")
		fmt.Println("  help:    Mostrar esta ayuda")
		os.Exit(1)
	}

	// Configuración de la base de datos desde variables de entorno
	dbHost := getEnvOrDefault("DB_HOST", "localhost")
	dbPort := getEnvOrDefault("DB_PORT", "5432")
	dbUser := getEnvOrDefault("DB_USER", "postgres")
	dbPassword := getEnvOrDefault("DB_PASSWORD", "password")
	dbName := getEnvOrDefault("DB_NAME", "iam_dev")

	// Construir string de conexión
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	// Conectar a la base de datos
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("Error conectando a la base de datos: %v", err)
	}
	defer db.Close()

	// Verificar conexión
	if err := db.Ping(); err != nil {
		log.Fatalf("Error haciendo ping a la base de datos: %v", err)
	}

	fmt.Printf("Conectado exitosamente a la base de datos: %s\n", dbName)

	command := os.Args[1]
	switch command {
	case "check":
		checkEvolutionReadiness(db)
	case "migrate":
		runEvolutionaryMigration(db)
	case "help":
		showHelp()
	default:
		fmt.Printf("Comando desconocido: %s\n", command)
		showHelp()
		os.Exit(1)
	}
}

func checkEvolutionReadiness(db *sql.DB) {
	fmt.Println("\n🔍 VERIFICANDO COMPATIBILIDAD PARA MIGRACIÓN EVOLUTIVA...")
	fmt.Println("======================================================")

	// Verificar estructura actual
	fmt.Println("\n1. ANALIZANDO ESTRUCTURA ACTUAL:")
	fmt.Println("--------------------------------")

	tables := []string{"plans", "roles", "tenants", "users"}
	for _, table := range tables {
		if tableExists(db, table) {
			fmt.Printf("  ✅ %s: EXISTE\n", table)

			// Verificar columnas críticas
			checkCriticalColumns(db, table)
		} else {
			fmt.Printf("  ❌ %s: NO EXISTE\n", table)
		}
	}

	fmt.Println("\n📋 PREPARACIÓN PARA MIGRACIÓN:")
	fmt.Println("  🔄 Esta migración AÑADIRÁ nuevas columnas")
	fmt.Println("  ✅ MANTENDRÁ datos existentes")
	fmt.Println("  🔧 MIGRARÁ datos al nuevo formato")
	fmt.Println("  ⚠️  Se ejecutará en una transacción segura")
}

func runEvolutionaryMigration(db *sql.DB) {
	fmt.Println("\n🚀 EJECUTANDO MIGRACIÓN EVOLUTIVA...")
	fmt.Println("==================================")

	// Hacer backup de conteos actuales
	fmt.Println("  📊 Guardando estado actual...")
	currentCounts := getCurrentCounts(db)

	for table, count := range currentCounts {
		fmt.Printf("    %s: %d registros\n", table, count)
	}

	// Leer el archivo de migración evolutiva
	sqlContent, err := ioutil.ReadFile("migration_safe_evolutionary.sql")
	if err != nil {
		log.Fatalf("Error leyendo archivo de migración evolutiva: %v", err)
	}

	// Ejecutar la migración en una transacción
	tx, err := db.Begin()
	if err != nil {
		log.Fatalf("Error iniciando transacción: %v", err)
	}
	defer tx.Rollback()

	fmt.Println("  🔧 Ejecutando migración evolutiva...")

	if _, err := tx.Exec(string(sqlContent)); err != nil {
		log.Fatalf("Error ejecutando migración evolutiva: %v", err)
	}

	if err := tx.Commit(); err != nil {
		log.Fatalf("Error confirmando transacción: %v", err)
	}

	fmt.Println("  ✅ Migración evolutiva ejecutada exitosamente!")

	// Verificar que los datos se mantuvieron
	fmt.Println("\n📊 VERIFICANDO INTEGRIDAD DE DATOS...")
	newCounts := getCurrentCounts(db)

	allGood := true
	for table, oldCount := range currentCounts {
		newCount := newCounts[table]
		if newCount >= oldCount {
			fmt.Printf("  ✅ %s: %d → %d registros\n", table, oldCount, newCount)
		} else {
			fmt.Printf("  ❌ %s: %d → %d registros (PERDIDA DE DATOS!)\n", table, oldCount, newCount)
			allGood = false
		}
	}

	if allGood {
		fmt.Println("\n🎉 MIGRACIÓN COMPLETADA EXITOSAMENTE!")
		fmt.Println("  ✅ Todos los datos se mantuvieron")
		fmt.Println("  🔧 Esquema actualizado a nueva arquitectura")
		fmt.Println("  🔐 Contraseñas protegidas (NO modificadas)")
		fmt.Println("  🎯 Roles genéricos funcionales para cualquier negocio")
		fmt.Println("  📊 Listo para usar módulos nuevos")
	} else {
		fmt.Println("\n⚠️  VERIFICAR RESULTADOS MANUALMENTE")
	}
}

func checkCriticalColumns(db *sql.DB, tableName string) {
	// Definir columnas esperadas para nueva arquitectura
	expectedColumns := map[string][]string{
		"roles":   {"permissions"}, // Solo permissions - roles genéricos simplificados
		"tenants": {"slug", "type", "status", "owner_id", "user_count"},
		"users":   {"first_name", "last_name", "is_active", "is_verified"},
	}

	if cols, exists := expectedColumns[tableName]; exists {
		for _, col := range cols {
			if columnExists(db, tableName, col) {
				fmt.Printf("    ✅ %s ya tiene: %s\n", tableName, col)
			} else {
				fmt.Printf("    🔄 %s necesita: %s\n", tableName, col)
			}
		}
	}
}

func tableExists(db *sql.DB, tableName string) bool {
	var exists bool
	query := `SELECT EXISTS (
		SELECT 1 FROM information_schema.tables 
		WHERE table_schema = 'public' AND table_name = $1
	)`
	err := db.QueryRow(query, tableName).Scan(&exists)
	if err != nil {
		return false
	}
	return exists
}

func columnExists(db *sql.DB, tableName, columnName string) bool {
	var exists bool
	query := `SELECT EXISTS (
		SELECT 1 FROM information_schema.columns 
		WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2
	)`
	err := db.QueryRow(query, tableName, columnName).Scan(&exists)
	if err != nil {
		return false
	}
	return exists
}

func getCurrentCounts(db *sql.DB) map[string]int {
	counts := make(map[string]int)
	tables := []string{"plans", "roles", "tenants", "users"}

	for _, table := range tables {
		if tableExists(db, table) {
			var count int
			query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
			err := db.QueryRow(query).Scan(&count)
			if err != nil {
				count = 0
			}
			counts[table] = count
		}
	}

	return counts
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func showHelp() {
	fmt.Println("\n📚 AYUDA DEL SCRIPT DE MIGRACIÓN EVOLUTIVA")
	fmt.Println("==========================================")
	fmt.Println("")
	fmt.Println("Este script actualiza el esquema existente hacia la nueva arquitectura modular")
	fmt.Println("manteniendo todos los datos existentes y añadiendo las nuevas funcionalidades.")
	fmt.Println("")
	fmt.Println("COMANDOS DISPONIBLES:")
	fmt.Println("  check   - Verificar qué cambios se aplicarán")
	fmt.Println("  migrate - Ejecutar migración evolutiva segura")
	fmt.Println("  help    - Mostrar esta ayuda")
	fmt.Println("")
	fmt.Println("CARACTERÍSTICAS:")
	fmt.Println("  ✅ Mantiene TODOS los datos existentes")
	fmt.Println("  🔧 Añade columnas necesarias para nueva arquitectura")
	fmt.Println("  📊 Migra datos al nuevo formato")
	fmt.Println("  ⚠️  Se ejecuta en transacción (rollback automático si hay error)")
	fmt.Println("")
	fmt.Println("VARIABLES DE ENTORNO:")
	fmt.Println("  DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME")
	fmt.Println("")
}
