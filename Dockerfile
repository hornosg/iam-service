# ==============================================
# IAM Service - Optimized Multi-stage Dockerfile
# ==============================================

# ==============================================
# Stage 1: Dependencies and cache optimization
# ==============================================
FROM golang:1.25-alpine AS deps
WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Configure private Go modules
ARG GITHUB_TOKEN
ENV GOPRIVATE=github.com/hornosg/*,github.com/mercadocercano/*
RUN if [ -n "$GITHUB_TOKEN" ]; then git config --global url."https://${GITHUB_TOKEN}@github.com/".insteadOf "https://github.com/"; fi


# Copy dependency files and download modules
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# ==============================================
# Stage 2: Build stage
# ==============================================
FROM deps AS builder

# Copy source code
COPY . .

# Build optimized binary with security hardening
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -trimpath \
    -o iam-service ./src/main.go

# Verify binary (skip on ARM64 Mac)
# RUN file iam-service && \
#     ldd iam-service 2>&1 | grep -q "not a dynamic executable" || exit 1

# ==============================================
# Stage 3: Development stage (with Air hot reload)
# ==============================================
FROM deps AS development

RUN apk add --no-cache curl && \
    go install github.com/air-verse/air@latest && \
    addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

# go.mod/go.sum ya copiados y módulos descargados en deps

# Create necessary directories and set permissions
RUN mkdir -p tmp scripts migrations logs /go/pkg/mod && \
    chmod -R 777 /go/pkg && \
    chown -R appuser:appgroup /app tmp scripts migrations logs

# Copy source code with correct ownership
COPY --chown=appuser:appgroup . .

# Switch to non-root user
USER appuser

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=40s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

EXPOSE 8080

# Entrypoint: configure git for private modules at runtime, then run Air
CMD sh -c 'if [ -n "$GITHUB_TOKEN" ]; then git config --global url."https://${GITHUB_TOKEN}@github.com/".insteadOf "https://github.com/"; fi && air -c .air.toml'

# ==============================================
# Stage 4: Production stage (Distroless)
# ==============================================
FROM gcr.io/distroless/static-debian12:nonroot AS production

# Metadata
LABEL org.opencontainers.image.title="IAM Service" \
      org.opencontainers.image.description="Multi-tenant Identity & Access Management" \
      org.opencontainers.image.source="https://github.com/saas-mt/iam-service" \
      org.opencontainers.image.vendor="SaaS MT Team" \
      org.opencontainers.image.licenses="MIT"

WORKDIR /app

# Copy binary and essential files only
COPY --from=builder --chown=nonroot:nonroot /app/iam-service ./
COPY --from=builder --chown=nonroot:nonroot /app/migrations ./migrations/

# Use distroless nonroot user (uid=65532)
USER nonroot

EXPOSE 8080

ENTRYPOINT ["./iam-service"]

# ==============================================
# Default stage: Development
# ==============================================
FROM development
