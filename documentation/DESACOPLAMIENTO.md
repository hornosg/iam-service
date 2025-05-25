# Estrategia de Desacoplamiento de Módulos IAM

## Objetivo

Este documento describe cómo los módulos del sistema IAM están diseñados para ser **completamente independientes** y cómo pueden ser fácilmente separados en microservicios en el futuro, reemplazando la inyección de dependencias actual por comunicación HTTP.

## Arquitectura Actual: Monolito Modular

### Estructura de Módulos
```
iam/src/
├── shared/           # Tipos e interfaces compartidas
├── api/             # Configuración API común
├── auth/            # Módulo de autenticación
├── user/            # Módulo de usuarios
├── tenant/          # Módulo de tenants
├── role/            # Módulo de roles
└── plan/            # Módulo de planes
```

### Patrón de Desacoplamiento

#### 1. Interfaces Compartidas (`shared/domain/service/`)

**Archivo: `shared/domain/service/user_finder.go`**
```go
type UserFinderService interface {
    FindUserByEmail(ctx context.Context, email string, tenantID *uuid.UUID) (*BasicUserData, error)
    FindUserByID(ctx context.Context, id uuid.UUID) (*BasicUserData, error)
}

type BasicUserData struct {
    ID           uuid.UUID
    Email        string
    PasswordHash string
    TenantID     uuid.UUID
    RoleID       uuid.UUID
    Status       string
    Provider     string
    FederatedID  string
}
```

#### 2. Implementación en Módulo User

**Archivo: `user/application/usecase/user_finder.go`**
```go
type UserFinderUseCase struct {
    userRepo port.UserRepository
}

// Implementa service.UserFinderService
func (uc *UserFinderUseCase) FindUserByEmail(ctx context.Context, email string, tenantID *uuid.UUID) (*service.BasicUserData, error) {
    user, err := uc.userRepo.GetByEmail(ctx, email, tenantID)
    if err != nil {
        return nil, err
    }
    
    return &service.BasicUserData{
        ID:           user.ID,
        Email:        user.Email.Value(),
        PasswordHash: user.PasswordHash,
        TenantID:     user.TenantID,
        RoleID:       user.RoleID,
        Status:       user.Status.String(),
        Provider:     user.Provider,
        FederatedID:  user.FederatedID,
    }, nil
}

// Verificación en tiempo de compilación
var _ service.UserFinderService = (*UserFinderUseCase)(nil)
```

#### 3. Uso en Módulo Auth

**Archivo: `auth/infrastructure/config/auth_module.go`**
```go
func SetupAuthModule(router *gin.RouterGroup, db *sql.DB, userService port.UserService, config AuthModuleConfig) {
    // userService implementa port.UserService (que es un alias de service.UserFinderService)
    loginUseCase := usecase.NewLoginUseCase(authConfig, authRepo, userService)
    // ... resto de la configuración
}
```

## Conexión Actual: Inyección de Dependencias

### Configuración en Main (Futuro)

**Archivo: `main.go` (estructura objetivo)**
```go
func main() {
    // 1. Configurar base de datos
    db := setupDatabase()
    
    // 2. Configurar módulos independientes
    userFinder := setupUserModule(db)
    
    // 3. Configurar módulo auth con dependencia inyectada
    setupAuthModule(router, db, userFinder)
    
    // 4. Otros módulos...
}

func setupUserModule(db *sql.DB) service.UserFinderService {
    userRepo := repository.NewPostgresUserRepository(db)
    return usecase.NewUserFinderUseCase(userRepo)
}
```

### Ventajas del Patrón Actual

1. **Type Safety**: Verificación en tiempo de compilación
2. **Performance**: Llamadas directas en memoria
3. **Transacciones**: Fácil manejo de transacciones ACID
4. **Debugging**: Stack traces completos
5. **Testing**: Fácil mocking de dependencias

## Migración a Microservicios: Gateway HTTP

### Estrategia de Migración

#### 1. Crear Cliente HTTP para User Service

**Archivo: `shared/infrastructure/gateway/user_service_client.go`**
```go
package gateway

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "iam/src/shared/domain/service"
    
    "github.com/google/uuid"
)

type UserServiceHTTPClient struct {
    baseURL    string
    httpClient *http.Client
}

func NewUserServiceHTTPClient(baseURL string) *UserServiceHTTPClient {
    return &UserServiceHTTPClient{
        baseURL:    baseURL,
        httpClient: &http.Client{Timeout: 30 * time.Second},
    }
}

// Implementa service.UserFinderService via HTTP
func (c *UserServiceHTTPClient) FindUserByEmail(ctx context.Context, email string, tenantID *uuid.UUID) (*service.BasicUserData, error) {
    url := fmt.Sprintf("%s/internal/users/by-email", c.baseURL)
    
    reqBody := map[string]interface{}{
        "email": email,
    }
    if tenantID != nil {
        reqBody["tenant_id"] = *tenantID
    }
    
    jsonBody, _ := json.Marshal(reqBody)
    req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
    if err != nil {
        return nil, err
    }
    
    req.Header.Set("Content-Type", "application/json")
    
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode == http.StatusNotFound {
        return nil, service.ErrUserNotFound
    }
    
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("user service error: %d", resp.StatusCode)
    }
    
    var userData service.BasicUserData
    if err := json.NewDecoder(resp.Body).Decode(&userData); err != nil {
        return nil, err
    }
    
    return &userData, nil
}

func (c *UserServiceHTTPClient) FindUserByID(ctx context.Context, id uuid.UUID) (*service.BasicUserData, error) {
    url := fmt.Sprintf("%s/internal/users/%s", c.baseURL, id)
    
    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, err
    }
    
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode == http.StatusNotFound {
        return nil, service.ErrUserNotFound
    }
    
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("user service error: %d", resp.StatusCode)
    }
    
    var userData service.BasicUserData
    if err := json.NewDecoder(resp.Body).Decode(&userData); err != nil {
        return nil, err
    }
    
    return &userData, nil
}

// Verificación en tiempo de compilación
var _ service.UserFinderService = (*UserServiceHTTPClient)(nil)
```

#### 2. Endpoints Internos en User Service

**Archivo: `user/infrastructure/controller/internal_handler.go`**
```go
package controller

import (
    "net/http"
    "iam/src/user/application/usecase"
    
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
)

type InternalUserHandler struct {
    userFinderUseCase *usecase.UserFinderUseCase
}

func NewInternalUserHandler(userFinderUseCase *usecase.UserFinderUseCase) *InternalUserHandler {
    return &InternalUserHandler{
        userFinderUseCase: userFinderUseCase,
    }
}

// POST /internal/users/by-email
func (h *InternalUserHandler) FindUserByEmail(c *gin.Context) {
    var req struct {
        Email    string     `json:"email" binding:"required"`
        TenantID *uuid.UUID `json:"tenant_id,omitempty"`
    }
    
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    user, err := h.userFinderUseCase.FindUserByEmail(c.Request.Context(), req.Email, req.TenantID)
    if err != nil {
        if err == service.ErrUserNotFound {
            c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
            return
        }
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, user)
}

// GET /internal/users/:id
func (h *InternalUserHandler) FindUserByID(c *gin.Context) {
    idStr := c.Param("id")
    id, err := uuid.Parse(idStr)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
        return
    }
    
    user, err := h.userFinderUseCase.FindUserByID(c.Request.Context(), id)
    if err != nil {
        if err == service.ErrUserNotFound {
            c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
            return
        }
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, user)
}

// RegisterInternalRoutes registra las rutas internas para comunicación entre microservicios
func (h *InternalUserHandler) RegisterInternalRoutes(router *gin.RouterGroup) {
    internal := router.Group("/internal/users")
    {
        internal.POST("/by-email", h.FindUserByEmail)
        internal.GET("/:id", h.FindUserByID)
    }
}
```

#### 3. Configuración por Feature Flags

**Archivo: `shared/config/microservices.go`**
```go
package config

import (
    "os"
    "iam/src/shared/domain/service"
    "iam/src/shared/infrastructure/gateway"
)

type MicroserviceConfig struct {
    UserServiceURL string
    AuthServiceURL string
    // ... otros servicios
}

func NewMicroserviceConfig() MicroserviceConfig {
    return MicroserviceConfig{
        UserServiceURL: getEnv("USER_SERVICE_URL", ""),
        AuthServiceURL: getEnv("AUTH_SERVICE_URL", ""),
    }
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}

// Factory para crear cliente local o remoto
func CreateUserFinderService(config MicroserviceConfig, localUserFinder service.UserFinderService) service.UserFinderService {
    if config.UserServiceURL != "" {
        // Modo microservicio: usar cliente HTTP
        return gateway.NewUserServiceHTTPClient(config.UserServiceURL)
    }
    // Modo monolito: usar implementación local
    return localUserFinder
}
```

#### 4. Main.go Adaptable

**Archivo: `main.go` (configuración híbrida)**
```go
func main() {
    // Configuración
    microConfig := config.NewMicroserviceConfig()
    
    // Setup según modo
    var userFinder service.UserFinderService
    
    if microConfig.UserServiceURL != "" {
        // Modo microservicio: Auth service independiente
        userFinder = gateway.NewUserServiceHTTPClient(microConfig.UserServiceURL)
        setupAuthServiceOnly(router, userFinder)
    } else {
        // Modo monolito: todos los módulos
        db := setupDatabase()
        localUserFinder := setupUserModule(db)
        userFinder = localUserFinder
        
        setupAllModules(router, db, userFinder)
    }
}
```

### Consideraciones de Migración

#### Manejo de Errores
```go
// En modo HTTP, mapear errores HTTP a errores de dominio
func mapHTTPError(statusCode int, body []byte) error {
    switch statusCode {
    case http.StatusNotFound:
        return service.ErrUserNotFound
    case http.StatusConflict:
        return service.ErrUserAlreadyExists
    case http.StatusBadRequest:
        return service.ErrInvalidUserData
    default:
        return fmt.Errorf("remote service error: %d - %s", statusCode, string(body))
    }
}
```

#### Circuit Breaker y Retry
```go
type ResilientUserServiceClient struct {
    client         *UserServiceHTTPClient
    circuitBreaker *CircuitBreaker
    retryPolicy    *RetryPolicy
}

func (c *ResilientUserServiceClient) FindUserByEmail(ctx context.Context, email string, tenantID *uuid.UUID) (*service.BasicUserData, error) {
    return c.circuitBreaker.Execute(func() (*service.BasicUserData, error) {
        return c.retryPolicy.Do(func() (*service.BasicUserData, error) {
            return c.client.FindUserByEmail(ctx, email, tenantID)
        })
    })
}
```

#### Observabilidad
```go
// Agregar tracing y métricas
func (c *UserServiceHTTPClient) FindUserByEmail(ctx context.Context, email string, tenantID *uuid.UUID) (*service.BasicUserData, error) {
    span, ctx := tracing.StartSpan(ctx, "user_service.find_by_email")
    defer span.Finish()
    
    start := time.Now()
    defer func() {
        metrics.RecordDuration("user_service_call", time.Since(start))
    }()
    
    // ... implementación HTTP
}
```

## Plan de Migración Gradual

### Fase 1: Preparación (Actual ✅)
- [x] Crear interfaces compartidas
- [x] Implementar módulos usando interfaces
- [x] Verificación en tiempo de compilación

### Fase 2: Infraestructura HTTP
- [ ] Crear clientes HTTP
- [ ] Implementar endpoints internos
- [ ] Configuración por feature flags

### Fase 3: Validación Híbrida
- [ ] Ejecutar ambos modos en paralelo
- [ ] Comparar resultados
- [ ] Ajustar diferencias

### Fase 4: Separación Completa
- [ ] Deployar servicios independientes
- [ ] Cambiar configuración a modo microservicio
- [ ] Remover código innecesario

## Ventajas de Esta Estrategia

1. **Zero Downtime**: Migración gradual sin interrupciones
2. **Rollback Fácil**: Cambio de configuración para volver atrás
3. **Misma Interfaz**: No cambios en la lógica de negocio
4. **Testeable**: Ambos modos pueden ejecutarse en paralelo
5. **Flexible**: Algunos módulos pueden ser remotos y otros locales

## Comandos de Verificación

### Verificar Implementación de Interfaces
```bash
go build ./... # Debe compilar sin errores
```

### Ejecutar Tests
```bash
go test ./shared/domain/service/... -v
go test ./user/application/usecase/... -v
go test ./auth/application/usecase/... -v
```

### Verificar Configuración
```bash
# Modo monolito
unset USER_SERVICE_URL
go run main.go

# Modo microservicio  
export USER_SERVICE_URL=http://user-service:8080
go run main.go
```

---

**Nota**: Esta estrategia garantiza que los módulos puedan separarse fácilmente en microservicios manteniendo la misma lógica de negocio y simplemente cambiando el mecanismo de comunicación de inyección de dependencias a HTTP. 