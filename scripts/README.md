# Scripts

Este directorio contiene scripts de utilidad para el servicio IAM.

## Scripts disponibles

### `wait-for-db.sh`
Script que espera a que PostgreSQL esté disponible antes de continuar. Usado en Docker Compose para garantizar que la base de datos esté lista antes de iniciar el servicio.

### `init-db.sh`
Script de inicialización de base de datos que ejecuta las migraciones necesarias. Se ejecuta automáticamente en el contenedor Docker para configurar el esquema inicial de la base de datos.

## Migraciones

Las migraciones SQL han sido movidas al directorio `/migrations` en la raíz del servicio. Los scripts de migración temporal han sido eliminados después de su ejecución exitosa.