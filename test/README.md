# Tests del Módulo IAM

Este directorio contiene la estructura completa de tests unitarios para el módulo IAM, siguiendo el patrón Object Mother y utilizando mocks para lograr una alta cobertura de código.

## Estructura de Directorios

```
iam/test/
├── user/
│   ├── domain/
│   │   └── entity/
│   │       └── user_mother.go          # Object Mother para User
│   ├── application/
│   │   └── usecase/
│   │       ├── create_user_test.go     # Tests para CreateUserUseCase
│   │       ├── update_user_test.go     # Tests para UpdateUserUseCase
│   │       └── delete_user_test.go     # Tests para DeleteUserUseCase
│   └── infrastructure/
│       └── persistence/
│           └── repository/
│               └── user_repository_mock.go  # Mock del repositorio User
├── tenant/
│   └── domain/
│       └── entity/
│           └── tenant_mother.go        # Object Mother para Tenant
├── role/
│   └── domain/
│       └── entity/
│           └── role_mother.go          # Object Mother para Role
├── auth/
│   ├── domain/
│   │   └── entity/
│   │       └── refresh_token_mother.go # Object Mother para RefreshToken
│   ├── application/
│   │   └── usecase/
│   │       ├── refresh_token_test.go   # Tests para RefreshTokenUseCase
│   │       └── logout_test.go          # Tests para LogoutUseCase
│   └── infrastructure/
│       └── persistence/
│           └── repository/
│               └── auth_repository_mock.go  # Mock del repositorio Auth
├── plan/
│   ├── domain/
│   │   └── entity/
│   │       └── plan_mother.go          # Object Mother para Plan
│   ├── application/
│   │   └── usecase/
│   │       └── create_plan_test.go     # Tests para CreatePlanUseCase
│   └── infrastructure/
│       └── persistence/
│           └── repository/
│               └── plan_repository_mock.go  # Mock del repositorio Plan
└── README.md                           # Este archivo
```

## Módulos Implementados

### 1. User (Usuario)
- **Entidad**: Usuario con email, password, tenant, rol, estado
- **Object Mother**: 15 métodos (WithDefaults, WithEmail, Pending, etc.)
- **Mock Repository**: CRUD completo con validaciones
- **Tests**: 29 casos (Create: 10, Update: 11, Delete: 8)

### 2. Tenant (Inquilino)
- **Entidad**: Tenant con nombre, tipo, límites, features
- **Object Mother**: 20 métodos (Startup, Business, Enterprise, etc.)
- **Tests**: Integrados en tests de User

### 3. Role (Rol)
- **Entidad**: Rol con permisos, tipo, tenant
- **Object Mother**: 18 métodos (SystemAdmin, TenantAdmin, User, etc.)
- **Tests**: Integrados en tests de User

### 4. Auth (Autenticación)
- **Entidad**: RefreshToken con expiración y usuario
- **Object Mother**: 12 métodos (Expired, ShortLived, LongLived, etc.)
- **Mock Repository**: Gestión de tokens y autenticación federada
- **Tests**: 8 casos (RefreshToken: 7, Logout: 4)

### 5. Plan (Planes de Suscripción)
- **Entidad**: Plan con tipo, precios, características, límites
- **Object Mother**: 15 métodos (Free, Basic, Premium, Enterprise, etc.)
- **Mock Repository**: CRUD con validaciones de nombre único
- **Tests**: 9 casos (CreatePlan: 9)

## Patrón Object Mother

El patrón Object Mother se utiliza para crear instancias de entidades de prueba con datos realistas y consistentes. Cada Object Mother proporciona:

### Métodos Básicos
- `WithDefaults()`: Crea una entidad con valores por defecto
- `WithID(id)`: Crea una entidad con un ID específico
- `Complete(...)`: Crea una entidad con todos los parámetros especificados

### Métodos de Conveniencia
- Métodos específicos para casos comunes (ej: `SystemAdmin()`, `Pending()`, `Expired()`)
- Métodos para configurar campos específicos (ej: `WithEmail()`, `WithTenant()`)

### Ejemplos de Uso por Módulo

#### User
```go
// Crear un usuario con valores por defecto
user := userMother.WithDefaults()

// Crear un usuario con email específico
user := userMother.WithEmail("test@example.com")

// Crear un usuario pendiente
user := userMother.Pending()
```

#### Auth
```go
// Crear un refresh token válido
token := tokenMother.WithDefaults()

// Crear un token expirado
token := tokenMother.Expired()

// Crear múltiples tokens para un usuario
tokens := tokenMother.ForUser(userID, 3)
```

#### Plan
```go
// Crear un plan básico
plan := planMother.Basic()

// Crear un plan enterprise
plan := planMother.Enterprise()

// Crear un plan con descuento específico
plan := planMother.WithYearlyDiscount(20.0)
```

## Mocks de Repositorio

Los mocks de repositorio implementan las interfaces de repositorio con funcionalidad en memoria para pruebas:

### Características Comunes
- **Thread-safe**: Utilizan mutex para operaciones concurrentes
- **Control de fallos**: Permiten simular errores específicos
- **Historial de llamadas**: Rastrean qué métodos se llamaron y cuántas veces
- **Datos realistas**: Mantienen índices y validaciones como un repositorio real

### Características Específicas

#### UserRepositoryMock
- Índice por email para búsquedas rápidas
- Validaciones de email único
- Soft delete (cambio de status)
- Búsquedas por tenant, rol, status

#### AuthRepositoryMock
- Gestión de refresh tokens por usuario
- Soporte para autenticación federada
- Limpieza automática de tokens expirados
- Índices para búsquedas eficientes

#### PlanRepositoryMock
- Índice por nombre para validación de unicidad
- Búsquedas por tipo y status
- Soft delete (cambio a deprecated)
- Paginación y conteo

### Métodos de Control
- `SetShouldFail(bool)`: Configura si todas las operaciones deberían fallar
- `ShouldFailOn(method)`: Configura un método específico para que falle
- `ResetFailures()`: Limpia todas las configuraciones de fallo
- `ResetCallHistory()`: Reinicia los contadores de llamadas
- `GetCallCount(method)`: Retorna cuántas veces se llamó un método

## Tests Unitarios

Los tests siguen el patrón AAA (Arrange, Act, Assert) y cubren:

### Casos de Éxito
- Operaciones normales con datos válidos
- Diferentes combinaciones de parámetros
- Casos límite válidos
- Integración entre múltiples Object Mothers

### Casos de Error
- Validaciones de entrada
- Errores de negocio (ej: email duplicado, plan existente)
- Fallos de infraestructura (ej: base de datos no disponible)
- Tokens expirados o inválidos

### Estructura de Test

```go
func TestCreateUserUseCase_Execute(t *testing.T) {
    // Arrange común
    mockRepo := repository.NewMockUserRepository()
    useCase := usecase.NewCreateUserUseCase(mockRepo)
    userMother := entity.Create()

    t.Run("debería crear un usuario con éxito", func(t *testing.T) {
        // Arrange específico
        mockRepo.ResetFailures()
        mockRepo.ResetCallHistory()
        req := &request.CreateUserRequest{...}

        // Act
        result, err := useCase.Execute(ctx, req)

        // Assert
        assert.NoError(t, err)
        assert.NotNil(t, result)
        assert.Equal(t, 1, mockRepo.GetCallCount("Create"))
    })
}
```

## Cobertura de Código

Los tests están diseñados para lograr alta cobertura:

- **Líneas de código**: Cubren todas las rutas de ejecución
- **Ramas**: Incluyen todos los casos if/else y switch
- **Funciones**: Prueban todos los métodos públicos
- **Casos límite**: Validan comportamientos extremos
- **Integración**: Combinan múltiples Object Mothers

## Ejecutar Tests

```bash
# Ejecutar todos los tests del módulo IAM
cd iam
go test ./test/...

# Ejecutar tests con cobertura
go test -cover ./test/...

# Ejecutar tests específicos por módulo
go test ./test/user/application/usecase/
go test ./test/auth/application/usecase/
go test ./test/plan/application/usecase/

# Ejecutar con verbose para ver detalles
go test -v ./test/...

# Ejecutar tests de un caso específico
go test -v ./test/user/application/usecase/ -run TestCreateUser
```

## Mejores Prácticas

1. **Aislamiento**: Cada test es independiente y no depende de otros
2. **Nombres descriptivos**: Los nombres de test describen claramente qué se está probando
3. **Datos realistas**: Los Object Mothers crean datos que reflejan casos reales
4. **Verificación completa**: Se verifican tanto los resultados como los efectos secundarios
5. **Limpieza**: Los mocks se resetean entre tests para evitar interferencias
6. **Composabilidad**: Los Object Mothers se pueden combinar para casos complejos

## Extensión

Para agregar nuevos tests:

1. **Nuevas entidades**: Crear Object Mother correspondiente siguiendo el patrón
2. **Nuevos casos de uso**: Agregar archivo de test siguiendo la estructura existente
3. **Nuevos repositorios**: Implementar mock con la misma funcionalidad
4. **Casos especiales**: Agregar métodos específicos a los Object Mothers
5. **Integración**: Combinar múltiples Object Mothers para casos complejos

## Estadísticas de Implementación

### Object Mothers Creados: 5
- **UserMother**: 15 métodos
- **TenantMother**: 20 métodos  
- **RoleMother**: 18 métodos
- **RefreshTokenMother**: 12 métodos
- **PlanMother**: 15 métodos

### Mocks de Repositorio: 3
- **MockUserRepository**: 14 métodos de interfaz + 8 auxiliares
- **MockAuthRepository**: 6 métodos de interfaz + 8 auxiliares
- **MockPlanRepository**: 11 métodos de interfaz + 6 auxiliares

### Tests Unitarios: 46 casos
- **User**: 29 casos (Create: 10, Update: 11, Delete: 8)
- **Auth**: 8 casos (RefreshToken: 7, Logout: 4)  
- **Plan**: 9 casos (CreatePlan: 9)

Esta estructura proporciona una base sólida para mantener alta calidad de código y facilitar el desarrollo dirigido por tests (TDD) en todos los módulos del sistema IAM. 