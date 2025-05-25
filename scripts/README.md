# Scripts de Migración Segura para IAM

## 🎯 Propósito

Estos scripts te permiten migrar la base de datos de forma **segura e idempotente**, evitando la acumulación de datos y garantizando que las migraciones se puedan ejecutar múltiples veces sin problemas.

## 📁 Archivos Incluidos

- `migration_safe.sql` - Script SQL idempotente con todas las tablas y datos iniciales
- `check_database_state.sql` - Script para verificar el estado actual de la BD
- `migrate.go` - Script en Go para ejecutar migraciones de forma automatizada
- `README.md` - Esta documentación

## 🛡️ Características de Seguridad

### ✅ Idempotencia Garantizada
- Todas las operaciones usan `IF NOT EXISTS` o `IF EXISTS` 
- Los datos solo se insertan si no existen ya
- Se puede ejecutar múltiples veces sin duplicar datos

### ✅ Verificaciones Inteligentes
- Verifica qué tablas ya existen antes de crearlas
- Cuenta registros existentes
- Detecta tipos enum, índices y triggers

### ✅ Transacciones Seguras
- Toda la migración se ejecuta en una transacción
- Si algo falla, se hace rollback automáticamente

## 🚀 Cómo Usar

### 1. Configurar Variables de Entorno

```bash
export DB_HOST=localhost
export DB_PORT=5432
export DB_USER=tu_usuario
export DB_PASSWORD=tu_contraseña
export DB_NAME=iam_dev
```

### 2. Verificar Estado Actual

```bash
# Opción 1: Con el script Go (recomendado)
cd scripts
go run migrate.go check

# Opción 2: Con psql directamente
psql -h localhost -U tu_usuario -d iam_dev -f check_database_state.sql
```

### 3. Ejecutar Migración Segura

```bash
# Opción 1: Con el script Go (recomendado)
cd scripts
go run migrate.go migrate

# Opción 2: Con psql directamente
psql -h localhost -U tu_usuario -d iam_dev -f migration_safe.sql
```

## 📊 Salida de Ejemplo

### Verificación de Estado
```
📊 VERIFICANDO ESTADO ACTUAL DE LA BASE DE DATOS...
================================================

1. TABLAS EXISTENTES:
--------------------
  plans: ✅ EXISTE
  roles: ❌ NO EXISTE
  tenants: ❌ NO EXISTE
  users: ✅ EXISTE

2. CANTIDAD DE REGISTROS:
-------------------------
  plans: 3 registros
  roles: tabla no existe
  tenants: tabla no existe
  users: 1 registros

📋 RECOMENDACIÓN:
  ⚠️  Algunas tablas faltan. Ejecuta 'go run migrate.go migrate' para crearlas.
```

### Después de Migración
```
🚀 EJECUTANDO MIGRACIÓN SEGURA...
===============================
  📝 Ejecutando script de migración...
  ✅ Migración ejecutada exitosamente!

📊 VERIFICANDO ESTADO DESPUÉS DE MIGRACIÓN...
  plans: ✅ EXISTE (3 registros)
  roles: ✅ EXISTE (3 registros)
  tenants: ✅ EXISTE (0 registros)
  users: ✅ EXISTE (1 registros)

📋 RECOMENDACIÓN:
  ✅ Todas las tablas existen. La migración es segura.
```

## 🔧 Comandos del Script Go

```bash
# Mostrar ayuda
go run migrate.go help

# Verificar estado actual
go run migrate.go check

# Ejecutar migración segura
go run migrate.go migrate
```

## 🎯 Casos de Uso

### ✅ Base de Datos Nueva
El script creará todas las tablas, índices, triggers y datos iniciales.

### ✅ Base de Datos Parcial
Solo creará las tablas/datos que falten, respetando los existentes.

### ✅ Base de Datos Completa
No hará cambios, solo confirmará que todo está correcto.

### ✅ Ejecutar Múltiples Veces
Siempre es seguro, nunca duplica datos ni estructuras.

## ⚠️ Notas Importantes

1. **Siempre verifica el estado antes** de hacer la migración
2. **Usa un backup** si tienes datos importantes
3. **Prueba en desarrollo** antes de aplicar en producción
4. **Las migraciones son idempotentes** - puedes ejecutarlas sin miedo

## 🗃️ Esquema de Tablas

### Plans
- Planes de suscripción para diferentes tipos de SaaS
- Datos iniciales: Plan Básico CRM, ERP, E-commerce

### Roles  
- Roles de sistema y de tenant
- Datos iniciales: super_admin, admin, user

### Tenants
- Inquilinos multi-tenant con configuraciones
- Inicia vacía, se puebla con el uso de la aplicación

### Users
- Usuarios del sistema con roles y tenants
- Inicia vacía, se puebla con registros

## 🔗 Dependencias

El script de Go requiere:
- Go 1.16+
- Paquete `github.com/lib/pq`

Para instalarlo:
```bash
go mod tidy
``` 