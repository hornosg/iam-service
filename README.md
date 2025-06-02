# saas-mt-iam-service

Este proyecto fue extraído del monorepo SaaS Marketplace como parte de la migración a repositorios independientes.

## Descripción

Servicio iam del ecosistema SaaS Marketplace.

## Tecnología

- **Tipo**: go
- **Lenguaje**: Go
- **Framework**: Gin/Fiber
- **Base de datos**: PostgreSQL

## Desarrollo

### Prerrequisitos

- Go 1.21+
- PostgreSQL 15+
- Docker (opcional)

### Instalación

```bash
# Clonar el repositorio
git clone https://github.com/trinityweb/saas-mt-iam-service.git
cd saas-mt-iam-service

# Instalar dependencias
go mod download

# Ejecutar
go run main.go
```

### Docker

```bash
# Construir imagen
docker build -t saas-mt-iam-service .

# Ejecutar contenedor
docker run -p 8080:8080 saas-mt-iam-service
```

## Configuración

Copia `.env.example` a `.env` y configura las variables necesarias.

## Documentación

- [Documentación de API](./api-docs/)
- [Documentación técnica](./documentation/)

## Migración desde Monorepo

Este proyecto fue extraído del monorepo original manteniendo todo su historial de git.

**Repositorio original**: https://github.com/trinityweb/saas-marketplace.git

## Contribución

1. Fork el proyecto
2. Crea una rama para tu feature
3. Commit tus cambios
4. Push a la rama
5. Abre un Pull Request

## Licencia

[Licencia del proyecto]
