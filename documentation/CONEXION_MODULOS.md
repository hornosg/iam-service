# Conexión de Módulos IAM - Estado Actual

## Diagrama de Arquitectura Modular

```
┌─────────────────────────────────────────────────────────────────┐
│                          main.go                                │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │              Inyección de Dependencias                      │ │
│  └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
                                    │
        ┌───────────────────────────┼───────────────────────────┐
        │                           │                           │
        ▼                           ▼                           ▼
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   API Module    │     │   Auth Module   │     │   User Module   │
│                 │     │                 │     │                 │
│ • Health Check  │     │ • Login         │     │ • CRUD Users    │
│ • OpenAPI Docs  │     │ • JWT Tokens    │     │ • UserFinder    │
│ • CORS          │     │ • OAuth Google  │     │   Service       │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                                    │               │
                                    │◄──────────────┘
                            shared/domain/service/
                            UserFinderService

        ┌───────────────────────────┼───────────────────────────┐
        │                           │                           │
        ▼                           ▼                           ▼
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Plan Module   │     │   Role Module   │     │  Tenant Module  │
│                 │     │                 │     │                 │
│ • CRUD Plans    │     │ • CRUD Roles    │     │ • CRUD Tenants  │
│ • Pricing       │     │ • Permissions   │     │ • Subscriptions │
│ • Features      │     │ • Access Control│     │ • Multi-tenancy │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

## Flujo de Conexión Actual

### 1. Configuración en Main (Objetivo)

```go
func main() {
    // Setup base
    db := setupDatabase()
    router := gin.Default()
    apiGroup := router.Group("/api/v1")
    
    // Configurar módulos en orden de dependencias
    userFinder := user.SetupUserModule(apiGroup, db)    // 1. User (independiente) ✅
    auth.SetupAuthModule(apiGroup, db, userFinder)      // 2. Auth (depende de User) ✅
}
```

### 2. Módulo User (Proveedor de Servicios) ✅ COMPLETADO

```go
func SetupUserModule(apiGroup *gin.RouterGroup, db *sql.DB) service.UserFinderService {
    // Crear repositorio PostgreSQL
    userRepo := repository.NewPostgresUserRepository(db)
    
    // Crear casos de uso
    createUser := usecase.NewCreateUserUseCase(userRepo)
    updateUser := usecase.NewUpdateUserUseCase(userRepo)
    getUser := usecase.NewGetUserByIDUseCase(userRepo)
    listUsers := usecase.NewListUsersUseCase(userRepo)
    deleteUser := usecase.NewDeleteUserUseCase(userRepo)
    userFinder := usecase.NewUserFinderUseCase(userRepo)    // Implementa UserFinderService
    
    // Configurar controlador HTTP
    userHandler := controller.NewUserHandler(createUser, updateUser, getUser, listUsers, deleteUser)
    userHandler.RegisterRoutes(apiGroup)  // POST /users, GET /users/{id}, etc.
    
    // Retornar interfaz para otros módulos
    return userFinder  // service.UserFinderService
}
```

### 3. Módulo Auth (Consumidor de Servicios) ✅ COMPLETADO

```go
func SetupAuthModule(apiGroup *gin.RouterGroup, db *sql.DB, userService service.UserFinderService) {
    // Crear repositorio auth
    authRepo := repository.NewPostgresAuthRepository(db)
    
    // Crear casos de uso CON dependencia inyectada
    loginUseCase := usecase.NewLoginUseCase(authConfig, authRepo, userService)  // ← Usa UserFinderService
    refreshUseCase := usecase.NewRefreshTokenUseCase(authConfig, authRepo, userService)
    validateUseCase := usecase.NewValidateTokenUseCase(authConfig)
    logoutUseCase := usecase.NewLogoutUseCase(authRepo)
    
    // Configurar controlador HTTP
    authHandler := controller.NewAuthHandler(loginUseCase, refreshUseCase, validateUseCase, logoutUseCase)
    authHandler.RegisterRoutes(apiGroup)  // POST /auth/login, POST /auth/refresh, etc.
}
```

## Interfaces de Desacoplamiento

### UserFinderService Interface

```go
// shared/domain/service/user_finder.go
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

### Implementación en User Module ✅

```go
// user/application/usecase/user_finder.go
type UserFinderUseCase struct {
    userRepo port.UserRepository
}

func (uc *UserFinderUseCase) FindUserByEmail(ctx context.Context, email string, tenantID *uuid.UUID) (*service.BasicUserData, error) {
    user, err := uc.userRepo.GetByEmail(ctx, email, tenantID)
    if err != nil {
        return nil, err
    }
    
    // Mapear de entidad User a BasicUserData
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

### Repositorio PostgreSQL ✅ IMPLEMENTADO

```go
// user/infrastructure/persistence/repository/postgres_user_repository.go
type PostgresUserRepository struct {
    db *sql.DB
}

// Implementa todas las operaciones:
// - Create, GetByID, GetByEmail, Update, Delete
// - GetByTenant, GetByStatus, GetByRole (con paginación)
// - ExistsByEmail, CountByTenant, CountByStatus
// - Manejo de errores específicos del dominio
// - Mapeo entre entidades y base de datos
```

### Uso en Auth Module ✅

```go
// auth/application/usecase/login.go
type LoginUseCase struct {
    config      AuthConfig
    authRepo    port.AuthRepository
    userService port.UserService  // ← Alias de service.UserFinderService
}

func (uc *LoginUseCase) Execute(ctx context.Context, req *request.LoginRequest) (*response.LoginResponse, error) {
    // Buscar usuario usando la interfaz
    user, err := uc.userService.FindUserByEmail(ctx, req.Email, req.TenantID)
    if err != nil {
        return nil, err
    }
    
    // Validar contraseña
    if !bcrypt.CheckPasswordHash(user.PasswordHash, req.Password) {
        return nil, exception.ErrInvalidCredentials
    }
    
    // Generar tokens JWT...
}
```

## Base de Datos

### Tabla Users ✅

```sql
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    tenant_id UUID NOT NULL,
    role_id UUID NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    provider VARCHAR(50) NOT NULL DEFAULT 'LOCAL',
    federated_id VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    CONSTRAINT users_email_tenant_unique UNIQUE (email, tenant_id),
    CONSTRAINT users_status_check CHECK (status IN ('ACTIVE', 'INACTIVE', 'PENDING', 'BLOCKED', 'DELETED'))
);
```

## Rutas HTTP Expuestas

### API Base ✅
- `GET /api/v1/health` - Health check
- `GET /docs/` - OpenAPI documentation

### Auth Module ✅
- `POST /api/v1/auth/login` - Login con email/password o Google OAuth
- `POST /api/v1/auth/refresh` - Renovar access token
- `POST /api/v1/auth/validate` - Validar token JWT  
- `POST /api/v1/auth/logout` - Logout (invalidar refresh token)

### User Module ✅
- `POST /api/v1/users` - Crear usuario
- `GET /api/v1/users/{id}` - Obtener usuario por ID
- `PUT /api/v1/users/{id}` - Actualizar usuario
- `DELETE /api/v1/users/{id}` - Eliminar usuario (soft delete)
- `GET /api/v1/users?tenant_id=...&status=...&page=1` - Listar usuarios con filtros

## Variables de Entorno

```bash
# Base de datos
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=iam_db

# Servidor
PORT=8080

# JWT
JWT_SECRET=your-super-secret-jwt-key

# OAuth
GOOGLE_CLIENT_ID=your-google-client-id
```

## Compilación y Verificación

```bash
# Verificar que compila sin errores
go build ./...

# Ejecutar tests
go test ./shared/domain/service/... -v
go test ./user/application/usecase/... -v
go test ./auth/application/usecase/... -v

# Verificar interfaces implementadas
go build -o /dev/null iam/src/user/infrastructure/persistence/repository
go build -o /dev/null iam/src/auth/infrastructure/config
```

## Estado Actual vs Objetivo

### Módulos Completados (100%)
- ✅ **Auth Module**: Sistema autenticación completo
- ✅ **User Module**: Gestión usuarios completa con PostgreSQL
- ✅ **Plan Module**: Gestión planes completa con PostgreSQL
- ✅ **Role Module**: Gestión roles completa con PostgreSQL ✅

### Módulos en Progreso
- 🔄 **Tenant Module (0%)**: Por implementar

## Rutas HTTP Planificadas

### Plan Module ✅ (COMPLETADO)
- `POST /api/v1/plans` - Crear plan ✅
- `GET /api/v1/plans/{id}` - Obtener plan por ID ✅
- `GET /api/v1/plans?active=true&page=1` - Listar planes con filtros ✅

### Role Module ✅ (COMPLETADO)
- `POST /api/v1/roles` - Crear rol ✅
- `GET /api/v1/roles/{id}` - Obtener rol por ID ✅
- `PUT /api/v1/roles/{id}` - Actualizar rol ✅
- `DELETE /api/v1/roles/{id}` - Eliminar rol ✅
- `GET /api/v1/roles?tenant_id=...&system=true&active=true` - Listar roles con filtros ✅

### Tenant Module 🔄 (Pendiente)
- `POST /api/v1/tenants` - Crear tenant
- `GET /api/v1/tenants/{id}` - Obtener tenant por ID
- `PUT /api/v1/tenants/{id}` - Actualizar tenant
- `GET /api/v1/tenants` - Listar tenants

## Base de Datos

### Tablas Implementadas ✅
- **users** - Usuarios del sistema
- **plans** - Planes de suscripción (con datos por defecto)
- **roles** - Roles y permisos (con roles de sistema por defecto)

### Tablas Pendientes 🔄
- **tenants** - Organizaciones/empresas
- **refresh_tokens** - Tokens de autenticación (ya implementado en auth)

## Dependencias entre Módulos

### Independientes (Sin dependencias)
- ✅ **Plan** - Completamente independiente
- ✅ **Auth** - Solo depende de User via interfaz
- ✅ **User** - Independiente (proveedor de servicios)

### Con Dependencias
- 🔄 **Role** - Puede depender de Tenant (roles por tenant)
- 🔄 **Tenant** - Puede depender de Plan (suscripciones)

## Próximos Pasos Inmediatos

### 🔄 Pendiente Inmediato
- [x] **Completar módulo Plan**: Repositorio + Configuración ✅ COMPLETADO
- [x] **Completar módulo Role**: Application + Infrastructure ✅ COMPLETADO
- [ ] **Implementar módulo Tenant**: Completo
- [ ] **Integración en main.go productivo**
- [ ] **Tests de integración**

---

**Estado**: ✅ **4 MÓDULOS COMPLETADOS** + 🔄 **1 MÓDULO EN PROGRESO** - Avanzando hacia arquitectura completa 