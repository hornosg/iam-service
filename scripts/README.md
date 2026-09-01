# Scripts

Este directorio contiene scripts de utilidad para el servicio IAM.

## Scripts disponibles

### `wait-for-db.sh`
Script que espera a que PostgreSQL esté disponible antes de continuar. Usado en Docker Compose para garantizar que la base de datos esté lista antes de iniciar el servicio.

## Migraciones

Las migraciones SQL han sido movidas al directorio `/migrations` en la raíz del servicio. Los scripts de migración temporal han sido eliminados después de su ejecución exitosa.