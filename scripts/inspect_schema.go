package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/lib/pq"
)

func main() {
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

	fmt.Printf("Conectado exitosamente a la base de datos: %s\n\n", dbName)

	tables := []string{"plans", "roles", "tenants", "users"}

	for _, table := range tables {
		inspectTable(db, table)
		fmt.Println()
	}
}

func inspectTable(db *sql.DB, tableName string) {
	fmt.Printf("🔍 ESTRUCTURA DE LA TABLA: %s\n", tableName)
	fmt.Println(strings.Repeat("=", 50))

	query := `
		SELECT 
			column_name,
			data_type,
			is_nullable,
			column_default
		FROM information_schema.columns 
		WHERE table_schema = 'public' 
		AND table_name = $1
		ORDER BY ordinal_position`

	rows, err := db.Query(query, tableName)
	if err != nil {
		fmt.Printf("Error consultando tabla %s: %v\n", tableName, err)
		return
	}
	defer rows.Close()

	fmt.Printf("%-20s %-15s %-10s %s\n", "COLUMNA", "TIPO", "NULLABLE", "DEFAULT")
	fmt.Printf("%-20s %-15s %-10s %s\n",
		strings.Repeat("-", 20),
		strings.Repeat("-", 15),
		strings.Repeat("-", 10),
		strings.Repeat("-", 20))

	for rows.Next() {
		var columnName, dataType, isNullable string
		var columnDefault sql.NullString

		err := rows.Scan(&columnName, &dataType, &isNullable, &columnDefault)
		if err != nil {
			fmt.Printf("Error escaneando fila: %v\n", err)
			continue
		}

		defaultValue := "NULL"
		if columnDefault.Valid {
			defaultValue = columnDefault.String
		}

		fmt.Printf("%-20s %-15s %-10s %s\n", columnName, dataType, isNullable, defaultValue)
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
