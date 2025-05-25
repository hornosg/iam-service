# Guía de Integración con IAM API - Arquitectura Modular

## API Base

La API está estructurada con arquitectura hexagonal modular. El servidor corre directamente en Go sin gateway intermedio.

- **URL Base**: `http://localhost:8080/api/v1`
- **Health Check**: `http://localhost:8080/health`
- **Documentación**: `http://localhost:8080/api-docs`

### Características del Servidor

1. **CORS**: Configurado para permitir:
   - Todos los orígenes (`*`)
   - Métodos: GET, POST, PUT, DELETE, OPTIONS
   - Headers: Content-Type, Authorization

2. **Middleware**: 
   - Compresión GZIP automática
   - Logging de requests
   - Manejo de errores estándar

3. **Arquitectura Modular**:
   - Auth Module: `/api/v1/auth/*`
   - User Module: `/api/v1/users/*`
   - Role Module: `/api/v1/roles/*`
   - Tenant Module: `/api/v1/tenants/*`
   - Plan Module: `/api/v1/plans/*`

## Autenticación

La API utiliza autenticación JWT. Para acceder a los endpoints protegidos:

1. Obtén el token mediante el endpoint `/auth/login`
2. Incluye el token en el header de las peticiones:
   ```
   Authorization: Bearer <tu_token>
   ```

### Flujo de Autenticación

1. **Login Local**:
   ```typescript
   const loginData = {
     email: "usuario@ejemplo.com",
     password: "contraseña",
     provider: "LOCAL",
     tenant_id: "uuid-del-tenant"
   };
   
   const response = await fetch('http://localhost:8080/api/v1/auth/login', {
     method: 'POST',
     headers: { 'Content-Type': 'application/json' },
     body: JSON.stringify(loginData)
   });
   ```

2. **Login con Google**:
   ```typescript
   const loginData = {
     email: "usuario@ejemplo.com",
     google_token: "token-de-google",
     provider: "GOOGLE",
     tenant_id: "uuid-del-tenant"
   };
   ```

3. **Refrescar Token**:
   ```typescript
   const response = await fetch('http://localhost:8080/api/v1/auth/refresh', {
     method: 'POST',
     headers: { 'Content-Type': 'application/json' },
     body: JSON.stringify({ refresh_token: "tu-refresh-token" })
   });
   ```

4. **Validar Token**:
   ```typescript
   const response = await fetch('http://localhost:8080/api/v1/auth/validate', {
     method: 'GET',
     headers: { 'Authorization': `Bearer ${token}` }
   });
   ```

5. **Logout**:
   ```typescript
   const response = await fetch('http://localhost:8080/api/v1/auth/logout', {
     method: 'POST',
     headers: { 'Authorization': `Bearer ${token}` }
   });
   ```

## Endpoints por Módulo

### Auth Module - `/api/v1/auth`
- `POST /login` - Autenticación de usuario
- `POST /refresh` - Refrescar token de acceso
- `GET /validate` - Validar token activo
- `POST /logout` - Cerrar sesión

### User Module - `/api/v1/users`
- `POST /` - Crear usuario
- `GET /:id` - Obtener usuario por ID
- `PUT /:id` - Actualizar usuario
- `DELETE /:id` - Eliminar usuario
- `GET /` - Listar usuarios (con filtros)

### Role Module - `/api/v1/roles`
- `POST /` - Crear rol
- `GET /:id` - Obtener rol por ID
- `PUT /:id` - Actualizar rol
- `DELETE /:id` - Eliminar rol
- `GET /` - Listar roles (con filtros)

### Tenant Module - `/api/v1/tenants`
- `POST /` - Crear tenant
- `GET /:id` - Obtener tenant por ID
- `GET /by-slug/:slug` - Obtener tenant por slug
- `PUT /:id` - Actualizar tenant
- `DELETE /:id` - Eliminar tenant
- `GET /` - Listar tenants (con filtros)
- `POST /:id/plan` - Asignar plan a tenant
- `DELETE /:id/plan` - Remover plan de tenant

### Plan Module - `/api/v1/plans`
- `POST /` - Crear plan (protegido)
- `GET /:id` - Obtener plan por ID
- `GET /` - Listar planes (público)

## Endpoints Públicos

Los siguientes endpoints **NO requieren autenticación**:

- `GET /plans` - Listar todos los planes
- `POST /tenants` - Crear nuevo tenant
- `GET /tenants/by-slug/:slug` - Buscar tenant por slug
- `POST /auth/login` - Autenticación
- `POST /auth/refresh` - Refrescar token

## Filtros y Paginación

La API utiliza un sistema unificado de filtros y paginación basado en el patrón Criteria. Todos los endpoints de listado soportan los mismos parámetros básicos y filtros específicos por entidad.

### Parámetros Básicos (todos los endpoints)

Todos los endpoints de listado soportan los siguientes parámetros:

- `page`: Número de página (default: 1)
- `page_size`: Tamaño de página (default: 10, max: 100)
- `sort_by`: Campo de ordenamiento (default: "created_at")
- `sort_dir`: Dirección de ordenamiento ("asc" o "desc", default: "desc")

### Estructura de Respuesta

Todos los endpoints de listado retornan la misma estructura:

```typescript
interface ListResponse<T> {
  items: T[];
  total_count: number;
  page: number;
  page_size: number;
  total_pages: number;
}
```

### Usuarios (`GET /users`)

```typescript
const params = new URLSearchParams({
  // Parámetros básicos
  page: "1",
  page_size: "20",
  sort_by: "email",
  sort_dir: "asc",
  
  // Filtros específicos de usuarios
  tenant_id: "uuid-del-tenant",
  status: "ACTIVE",
  role_id: "uuid-del-rol",
  email: "john",              // Búsqueda LIKE
  first_name: "Juan",         // Búsqueda LIKE
  last_name: "Pérez",         // Búsqueda LIKE
  provider: "LOCAL"           // LOCAL o GOOGLE
});

const response = await fetch(`http://localhost:8080/api/v1/users?${params}`);
```

**Filtros disponibles para usuarios:**
- `tenant_id`: UUID del tenant
- `status`: Estado (ACTIVE, INACTIVE, SUSPENDED, DELETED)
- `role_id`: UUID del rol
- `email`: Búsqueda por email (LIKE)
- `first_name`: Búsqueda por nombre (LIKE)
- `last_name`: Búsqueda por apellido (LIKE)
- `provider`: Proveedor (LOCAL, GOOGLE)

### Roles (`GET /roles`)

```typescript
const params = new URLSearchParams({
  // Parámetros básicos
  page: "1",
  page_size: "10",
  sort_by: "name",
  sort_dir: "asc",
  
  // Filtros específicos de roles
  tenant_id: "uuid-del-tenant",
  type: "TENANT",
  status: "ACTIVE",
  name: "admin",              // Búsqueda LIKE
  
  // Filtros especiales
  system: "true",             // Solo roles de sistema
  active: "true"              // Solo roles activos
});

const response = await fetch(`http://localhost:8080/api/v1/roles?${params}`);
```

**Filtros disponibles para roles:**
- `tenant_id`: UUID del tenant
- `type`: Tipo (SYSTEM, TENANT)
- `status`: Estado (ACTIVE, INACTIVE)
- `name`: Búsqueda por nombre (LIKE)
- `system`: Solo roles de sistema ("true")
- `active`: Solo roles activos ("true")

### Tenants (`GET /tenants`)

```typescript
const params = new URLSearchParams({
  // Parámetros básicos
  page: "2",
  page_size: "50",
  sort_by: "created_at",
  sort_dir: "desc",
  
  // Filtros específicos de tenants
  owner_id: "uuid-del-propietario",
  status: "ACTIVE",
  type: "BUSINESS",
  plan_id: "uuid-del-plan",
  name: "acme",               // Búsqueda LIKE
  slug: "mi-empresa",         // Búsqueda LIKE
  domain: "miempresa.com",    // Búsqueda LIKE
  
  // Filtros especiales
  active: "true"              // Solo tenants activos
});

const response = await fetch(`http://localhost:8080/api/v1/tenants?${params}`);
```

**Filtros disponibles para tenants:**
- `owner_id`: UUID del propietario
- `status`: Estado (ACTIVE, INACTIVE, SUSPENDED)
- `type`: Tipo (PERSONAL, STARTUP, BUSINESS, ENTERPRISE)
- `plan_id`: UUID del plan
- `name`: Búsqueda por nombre (LIKE)
- `slug`: Búsqueda por slug (LIKE)
- `domain`: Búsqueda por dominio (LIKE)
- `active`: Solo tenants activos ("true")

### Planes (`GET /plans`)

```typescript
const params = new URLSearchParams({
  // Parámetros básicos
  page: "1",
  page_size: "10",
  sort_by: "price",
  sort_dir: "asc",
  
  // Filtros específicos de planes
  type: "PROFESSIONAL",
  status: "ACTIVE",
  name: "pro",                // Búsqueda LIKE
  currency: "USD",
  min_price: "10.00",         // Precio mínimo
  max_price: "100.00",        // Precio máximo
  
  // Filtros especiales
  active: "true"              // Solo planes activos
});

const response = await fetch(`http://localhost:8080/api/v1/plans?${params}`);
```

**Filtros disponibles para planes:**
- `type`: Tipo (FREE, STARTER, PROFESSIONAL, ENTERPRISE)
- `status`: Estado (ACTIVE, INACTIVE)
- `name`: Búsqueda por nombre (LIKE)
- `currency`: Moneda
- `min_price`: Precio mínimo
- `max_price`: Precio máximo
- `active`: Solo planes activos ("true")

### Ejemplos de Búsquedas Complejas

#### Usuarios activos de un tenant específico ordenados por email:
```bash
GET /api/v1/users?tenant_id=123e4567-e89b-12d3-a456-426614174000&status=ACTIVE&sort_by=email&sort_dir=asc
```

#### Roles de sistema paginados:
```bash
GET /api/v1/roles?system=true&page=2&page_size=20
```

#### Tenants de tipo BUSINESS con plan específico:
```bash
GET /api/v1/tenants?type=BUSINESS&plan_id=456e7890-e89b-12d3-a456-426614174000&active=true
```

#### Planes profesionales en rango de precios:
```bash
GET /api/v1/plans?type=PROFESSIONAL&min_price=50&max_price=200&currency=USD
```

## Manejo de Errores

La API retorna errores en el siguiente formato:

```json
{
  "error": "Descripción del error"
}
```

### Códigos de Estado Comunes:
- `200` - Éxito
- `201` - Creado exitosamente
- `204` - Éxito sin contenido
- `400` - Datos de entrada inválidos
- `401` - No autorizado / Token inválido
- `403` - Prohibido / Sin permisos
- `404` - Recurso no encontrado
- `409` - Conflicto (recurso ya existe)
- `500` - Error interno del servidor

## Cliente API TypeScript Completo

```typescript
class IAMApiClient {
  private baseUrl = 'http://localhost:8080/api/v1';
  private token: string | null = null;

  setToken(token: string) {
    this.token = token;
  }

  private async fetch(endpoint: string, options: RequestInit = {}) {
    const headers = {
      'Content-Type': 'application/json',
      ...(this.token ? { Authorization: `Bearer ${this.token}` } : {}),
      ...options.headers
    };

    const response = await fetch(`${this.baseUrl}${endpoint}`, {
      ...options,
      headers
    });

    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.error || 'Unknown error');
    }

    // No parsear JSON para respuestas 204
    if (response.status === 204) {
      return {};
    }

    return response.json();
  }

  // Auth Module
  async login(data: LoginRequest) {
    return this.fetch('/auth/login', {
      method: 'POST',
      body: JSON.stringify(data)
    });
  }

  async refreshToken(refreshToken: string) {
    return this.fetch('/auth/refresh', {
      method: 'POST',
      body: JSON.stringify({ refresh_token: refreshToken })
    });
  }

  async validateToken() {
    return this.fetch('/auth/validate');
  }

  async logout() {
    return this.fetch('/auth/logout', { method: 'POST' });
  }

  // User Module
  async createUser(data: CreateUserRequest) {
    return this.fetch('/users', {
      method: 'POST',
      body: JSON.stringify(data)
    });
  }

  async getUserById(id: string) {
    return this.fetch(`/users/${id}`);
  }

  async updateUser(id: string, data: UpdateUserRequest) {
    return this.fetch(`/users/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data)
    });
  }

  async deleteUser(id: string) {
    return this.fetch(`/users/${id}`, { method: 'DELETE' });
  }

  async listUsers(params?: UserListParams) {
    const query = params ? '?' + new URLSearchParams(params as any).toString() : '';
    return this.fetch(`/users${query}`);
  }

  // Role Module
  async createRole(data: CreateRoleRequest) {
    return this.fetch('/roles', {
      method: 'POST',
      body: JSON.stringify(data)
    });
  }

  async getRoleById(id: string) {
    return this.fetch(`/roles/${id}`);
  }

  async updateRole(id: string, data: UpdateRoleRequest) {
    return this.fetch(`/roles/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data)
    });
  }

  async deleteRole(id: string) {
    return this.fetch(`/roles/${id}`, { method: 'DELETE' });
  }

  async listRoles(params?: RoleListParams) {
    const query = params ? '?' + new URLSearchParams(params as any).toString() : '';
    return this.fetch(`/roles${query}`);
  }

  // Tenant Module
  async createTenant(data: CreateTenantRequest) {
    return this.fetch('/tenants', {
      method: 'POST',
      body: JSON.stringify(data)
    });
  }

  async getTenantById(id: string) {
    return this.fetch(`/tenants/${id}`);
  }

  async getTenantBySlug(slug: string) {
    return this.fetch(`/tenants/by-slug/${slug}`);
  }

  async updateTenant(id: string, data: UpdateTenantRequest) {
    return this.fetch(`/tenants/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data)
    });
  }

  async deleteTenant(id: string) {
    return this.fetch(`/tenants/${id}`, { method: 'DELETE' });
  }

  async listTenants(params?: TenantListParams) {
    const query = params ? '?' + new URLSearchParams(params as any).toString() : '';
    return this.fetch(`/tenants${query}`);
  }

  async setTenantPlan(tenantId: string, data: SetPlanRequest) {
    return this.fetch(`/tenants/${tenantId}/plan`, {
      method: 'POST',
      body: JSON.stringify(data)
    });
  }

  async removeTenantPlan(tenantId: string) {
    return this.fetch(`/tenants/${tenantId}/plan`, { method: 'DELETE' });
  }

  // Plan Module
  async createPlan(data: CreatePlanRequest) {
    return this.fetch('/plans', {
      method: 'POST',
      body: JSON.stringify(data)
    });
  }

  async getPlanById(id: string) {
    return this.fetch(`/plans/${id}`);
  }

  async listPlans(params?: PlanListParams) {
    const query = params ? '?' + new URLSearchParams(params as any).toString() : '';
    return this.fetch(`/plans${query}`);
  }
}
```

## Tipos TypeScript de Ejemplo

```typescript
// Auth Types
interface LoginRequest {
  email: string;
  password?: string;
  google_token?: string;
  provider: 'LOCAL' | 'GOOGLE';
  tenant_id: string;
}

interface LoginResponse {
  access_token: string;
  refresh_token: string;
  user: UserResponse;
}

// User Types
interface CreateUserRequest {
  email: string;
  password: string;
  first_name: string;
  last_name: string;
  role_id: string;
  tenant_id: string;
  provider?: 'LOCAL' | 'GOOGLE';
}

interface UpdateUserRequest {
  email?: string;
  first_name?: string;
  last_name?: string;
  role_id?: string;
  status?: 'ACTIVE' | 'INACTIVE' | 'SUSPENDED';
}

interface UserListParams {
  // Parámetros básicos
  page?: string;
  page_size?: string;
  sort_by?: string;
  sort_dir?: string;
  
  // Filtros específicos de usuarios
  tenant_id?: string;
  status?: string;
  role_id?: string;
  email?: string;
  first_name?: string;
  last_name?: string;
  provider?: string;
}

// Role Types  
interface CreateRoleRequest {
  name: string;
  description?: string;
  permissions: string[];
  type: 'SYSTEM' | 'TENANT';
  tenant_id?: string;
}

interface UpdateRoleRequest {
  name?: string;
  description?: string;
  permissions?: string[];
  status?: 'ACTIVE' | 'INACTIVE';
}

interface RoleListParams {
  // Parámetros básicos
  page?: string;
  page_size?: string;
  sort_by?: string;
  sort_dir?: string;
  
  // Filtros específicos de roles
  tenant_id?: string;
  type?: string;
  status?: string;
  name?: string;
  system?: string;
  active?: string;
}

// Tenant Types
interface CreateTenantRequest {
  name: string;
  slug: string;
  domain?: string;
  type: 'PERSONAL' | 'STARTUP' | 'BUSINESS' | 'ENTERPRISE';
  owner_id: string;
  settings?: Record<string, any>;
}

interface UpdateTenantRequest {
  name?: string;
  domain?: string;
  type?: 'PERSONAL' | 'STARTUP' | 'BUSINESS' | 'ENTERPRISE';
  status?: 'ACTIVE' | 'INACTIVE' | 'SUSPENDED';
  settings?: Record<string, any>;
}

interface TenantListParams {
  // Parámetros básicos
  page?: string;
  page_size?: string;
  sort_by?: string;
  sort_dir?: string;
  
  // Filtros específicos de tenants
  owner_id?: string;
  status?: string;
  type?: string;
  plan_id?: string;
  name?: string;
  slug?: string;
  domain?: string;
  active?: string;
}

interface SetPlanRequest {
  plan_id: string;
  expires_at?: string;
}

// Plan Types
interface CreatePlanRequest {
  name: string;
  description?: string;
  type: 'FREE' | 'STARTER' | 'PROFESSIONAL' | 'ENTERPRISE';
  price: number;
  currency: string;
  features: string[];
  limits: Record<string, any>;
}

interface PlanListParams {
  // Parámetros básicos
  page?: string;
  page_size?: string;
  sort_by?: string;
  sort_dir?: string;
  
  // Filtros específicos de planes
  type?: string;
  status?: string;
  name?: string;
  currency?: string;
  min_price?: string;
  max_price?: string;
  active?: string;
}
```

## Consideraciones Importantes

1. **Manejo de Tokens**:
   - Access tokens expiran en 15 minutos
   - Refresh tokens expiran en 7 días
   - Implementa un interceptor para refrescar automáticamente

2. **Multi-tenancy**:
   - Todos los recursos están asociados a un tenant
   - Algunos endpoints requieren tenant_id en query params

3. **Validaciones**:
   - Valida los datos antes de enviarlos a la API
   - Los UUIDs deben estar en formato válido

4. **Roles y Permisos**:
   - Roles de sistema no pueden modificarse/eliminarse
   - Permisos son arrays de strings

5. **Paginación**:
   - Página por defecto: 1
   - Tamaño por defecto: 10
   - Máximo por página: 100

6. **Rate Limiting**:
   - Implementar en el cliente si es necesario
   - Respetar códigos de estado HTTP 429

Para más detalles técnicos, consulta la especificación OpenAPI completa disponible en `/api-docs`.
