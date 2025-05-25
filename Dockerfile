# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copiar archivos de dependencias
COPY go.mod go.sum ./
RUN go mod download

# Copiar el código fuente
COPY . .

# Compilar la aplicación
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./src/main.go

# Final stage
FROM alpine:3.18

# Variables de entorno configurables
ARG APP_VERSION=1.0
ARG APP_PORT=8080
ARG APP_ENV=production

# Añadir etiquetas para mejor gestión desde Terraform
LABEL maintainer="SaaS Team" \
      application="iam" \
      version="${APP_VERSION}" \
      description="Multi-tenant IAM service" \
      environment="${APP_ENV}"

# Definir variables de entorno
ENV PORT=${APP_PORT} \
    APP_ENV=${APP_ENV}

# Instalar dependencias necesarias en una sola capa
RUN apk add --no-cache postgresql-client dos2unix ca-certificates tzdata && \
    cp /usr/share/zoneinfo/UTC /etc/localtime && \
    echo "UTC" > /etc/timezone && \
    adduser -D appuser

WORKDIR /app

# Copiar el binario compilado desde el stage anterior
COPY --from=builder /app/main .

# Copiar todas las migraciones disponibles
COPY --from=builder /app/migrations ./migrations
COPY --from=builder /app/scripts ./scripts
COPY --from=builder /app/src/tenant/infrastructure/persistence/migrations ./migrations/tenant

# Asignar permisos adecuados
RUN chmod +x /app/main && \
    chown -R appuser:appuser /app

# Cambiar al usuario no-root para seguridad
USER appuser

# Exponer el puerto (configurable a través de ARG)
EXPOSE ${PORT}

# Configurar healthcheck para que Terraform sepa si el servicio está listo
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
  CMD wget --quiet --tries=1 --spider http://localhost:${PORT}/health || exit 1

# Comando para ejecutar la aplicación
CMD ["./main"]
