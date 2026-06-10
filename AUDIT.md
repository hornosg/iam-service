# IAM Service — Reporte de Auditoría Hexagonal

**Fecha de inicio:** 2026-06-09  
**Fecha de cierre:** 2026-06-10  
**Auditor:** Claude Code (go-hex-audit skill)  
**Branch auditado:** `master`  
**Commit de cierre:** `38aa93b`

---

## Resumen ejecutivo

La auditoría cubrió arquitectura hexagonal, seguridad de dominio, separación de capas, cobertura de tests y pipelines e2e. Se encontraron **11 findings** (3 HIGH, 5 MEDIUM, 3 LOW). Todos los HIGH y MEDIUM fueron resueltos; 1 LOW fue descartado intencionalmente.

| Severidad | Total | Resuelto | Descartado | Pendiente |
|-----------|-------|----------|------------|-----------|
| HIGH      | 3     | 3        | 0          | 0         |
| MEDIUM    | 5     | 5        | 0          | 0         |
| LOW       | 3     | 2        | 1          | 0         |
| **Total** | **11**| **10**   | **1**      | **0**     |

---

## Findings detallados

### HIGH

#### Fix #1 — Tags GORM en entidad de dominio
- **Archivo:** `auth/domain/entity/refresh_token.go`
- **Problema:** La entidad `RefreshToken` tenía tags `gorm:` directamente en campos del dominio, acoplando el dominio a la infraestructura de persistencia.
- **Fix:** Tags GORM eliminados. La entidad de dominio es ahora pura.
- **Estado:** ✅ Resuelto

#### Fix #2 — `PasswordHash` expuesto en respuesta de listado de usuarios
- **Archivo:** `user/application/usecase/list_users_by_criteria.go`
- **Problema:** El use case retornaba `[]*entity.User` directamente, exponiendo `PasswordHash` en la respuesta HTTP.
- **Fix:** Retorna `[]*response.UserResponse` — el hash nunca llega al controller.
- **Estado:** ✅ Resuelto

#### Fix #7 — Paquete zombie `src/domain/`
- **Archivo:** `src/domain/` (directorio completo)
- **Problema:** Directorio de dominio global no utilizado, remanente de una arquitectura anterior. Creaba confusión sobre cuál era el dominio canónico.
- **Fix:** Directorio eliminado.
- **Estado:** ✅ Resuelto

---

### MEDIUM

#### Fix #3 — Logger hardcodeado en `revoke_all.go`
- **Archivo:** `login/logout/revoke_all.go`
- **Problema:** Logger inyectado como dependencia concreta de infraestructura en lugar de usar el port de dominio.
- **Fix:** Logger inyectado via `sharedport.SecurityEventLogger`.
- **Estado:** ✅ Resuelto

#### Fix #4 — `api/monitoring` importado en application layer
- **Archivo:** `tenant/application/usecase/create_tenant.go`
- **Problema:** El use case importaba directamente el paquete `api/monitoring` (infraestructura), violando la regla de dependencia hexagonal.
- **Fix:** Reemplazado por `sharedport.MetricsRecorder` — el use case depende del port, no de la implementación.
- **Estado:** ✅ Resuelto

#### Fix #5 — Dependencia cross-module auth↔tenant en dominio
- **Archivo:** `auth/domain/value_object/token_claims.go`
- **Problema:** El módulo `auth` importaba directamente tipos del módulo `tenant` en capa de dominio, creando acoplamiento prohibido entre bounded contexts.
- **Fix:** ACL (Anti-Corruption Layer) creada en `auth/infrastructure/adapter/tenant_features_adapter.go`. Los módulos se comunican solo a través de interfaces de port.
- **Estado:** ✅ Resuelto

#### Fix #6 — `http.Client` y `GoogleClientID` en application layer
- **Archivo:** `auth/application/usecase/login.go`
- **Problema:** El use case de login contenía un `http.Client` y el `GoogleClientID` directamente en la capa de application, acoplándola a la infraestructura HTTP de Google.
- **Fix:** Port `auth/domain/port/google_token_verifier.go` creado. Implementación HTTP en `auth/infrastructure/adapter/google_token_verifier.go`. El use case solo conoce el port.
- **Estado:** ✅ Resuelto

#### Fix #8 — Handler HTTP duplicado en user module
- **Archivo:** `user/infrastructure/controller/http_handler.go`
- **Problema:** Existían dos handlers HTTP para el módulo user, generando ambigüedad en el routing y código muerto.
- **Fix:** Handler duplicado eliminado; se conserva `RefactoredUserHandler`.
- **Estado:** ✅ Resuelto

---

### LOW

#### Fix #9 — Migración `shared/` → `go-shared`
- **Contexto:** El paquete `src/shared/` estaba duplicando código que debería ser un módulo compartido del ecosistema.
- **Fix:** Repo `mercadocercano/go-shared` publicado en GitHub (tag `v0.1.0`). `iam-service/go.mod` migrado a `github.com/mercadocercano/go-shared`. Directorio `src/shared/` eliminado de iam-service.
- **Estado:** ✅ Resuelto

#### Fix #10 — Import `golang-jwt` en dominio vía `TokenClaims`
- **Archivo:** `auth/domain/value_object/token_claims.go`
- **Problema:** `TokenClaims` importaba `golang-jwt` directamente en el dominio para embed `jwt.RegisteredClaims`.
- **Fix:** `TokenClaims` limpiado — sin import jwt. `JWTClaims` wrapper creado en `auth/infrastructure/adapter/jwt_claims.go`. `JWTService` port en `auth/domain/port/`. El dominio no tiene dependencias de JWT.
- **Estado:** ✅ Resuelto

#### Fix #11 — `encoding/json` en entidad de dominio
- **Archivo:** `user/domain/value_object/email.go`
- **Problema:** El value object `Email` importa `encoding/json` para implementar `MarshalJSON`/`UnmarshalJSON`.
- **Análisis:** `encoding/json` es stdlib, no infraestructura externa. Los métodos `MarshalJSON`/`UnmarshalJSON` deben vivir en dominio porque el campo `value` es privado y `UnmarshalJSON` ejecuta validación de dominio. Moverlos a infraestructura rompería el encapsulamiento.
- **Estado:** ⚪ Descartado intencionalmente (decisión técnica justificada)

---

## Cambios estructurales adicionales

### CI/CD
- `.github/workflows/deploy.yml` actualizado con `GOPRIVATE` y `GONOSUMDB` para `github.com/mercadocercano`.
- Secret `GO_PRIVATE_TOKEN` configurado en GitHub Actions.
- Dockerfile ya tenía `GOPRIVATE` y el `GITHUB_TOKEN` build-arg.

### OpenAPI
- `api-docs/openapi.yaml` actualizado para reflejar todos los endpoints activos tras los refactors.

---

## Cobertura de tests

| Fecha      | Cobertura total | Threshold |
|------------|-----------------|-----------|
| 2026-06-09 | 3.5%            | 80%       |
| 2026-06-10 | **83.4%**       | 80% ✅    |

**Tests escritos en esta auditoría** (7 archivos, ~805 líneas):

| Archivo | Use case cubierto |
|---------|-------------------|
| `test/plan/application/usecase/get_plan_by_id_test.go` | `GetPlanByIDUseCase` |
| `test/plan/application/usecase/list_plans_test.go` | `ListPlansUseCase` + `GetActive` |
| `test/tenant/application/usecase/list_tenants_test.go` | `ListTenantsUseCase` (5 métodos) |
| `test/tenant/application/usecase/set_plan_test.go` | `SetPlanUseCase` + `RemovePlan` |
| `test/tenant/application/usecase/get_tenant_features_test.go` | `GetTenantFeaturesUseCase` |
| `test/tenant/application/usecase/update_tenant_features_test.go` | `UpdateTenantFeaturesUseCase` |
| `test/user/application/usecase/user_finder_test.go` | `UserFinderUseCase` |

**Cobertura de dominio por módulo** (al cierre):

| Módulo | Dominio | Application |
|--------|---------|-------------|
| auth   | ~80%    | ~85%        |
| plan   | ~97%    | ~100%       |
| role   | ~95%    | ~85%        |
| tenant | ~95%    | ~85%        |
| user   | ~90%    | ~85%        |

---

## Suite e2e Newman

- **Colección:** `postman/collection.json` (10 carpetas, 43 requests)
- **Ambiente:** `postman/environment.local.json`
- **Script:** `scripts/e2e.sh` — Postgres efímero (docker-compose.e2e.yml, tmpfs, puerto 5433), build Go, Newman
- **Resultado:** 43/43 requests ✅ · 82/82 assertions ✅ · 0 failures
- **Cobertura funcional:** Health, Auth (login/refresh/validate/logout/revoke-all), Plans CRUD, Roles CRUD, Tenants CRUD + plan + features, Users CRUD, Multi-tenant isolation (8 casos), Negative cases

---

## ADRs creados

| ADR | Título | Impacto |
|-----|--------|---------|
| [ADR-001](../../../docs/adr/ADR-001-canonical-logs.md) | Canonical Logs | Patrón estructurado para todos los logs del ecosistema |
| [ADR-001a](../../../docs/adr/ADR-001a-security-events.md) | Security Events | Extensión de canonical logs para eventos de seguridad |
| [ADR-002](../../../docs/adr/ADR-002-metrics-recording.md) | MetricsRecorder con Kind+Unit | Interfaz port para métricas desacoplada de Prometheus |

---

## Deuda técnica conocida al cierre

| Área | Descripción | Impacto |
|------|-------------|---------|
| Cobertura infra | Controllers, repositories y adapters a 0% | Bajo — cubiertos por e2e Newman |
| `GetActiveRoles` | `role/application/usecase/list_roles.go:69` a 0% | Muy bajo — método auxiliar raramente invocado |
| `NewTenantSummaryResponse` | `tenant/application/response` a 0% | Muy bajo — helper de presentación |
| Validación `max_users` en `CreatePlanRequest` | No valida que `max_users` sea consistente con el tipo | Low — el dominio calcula el valor desde el tipo |

---

## Arquitectura al cierre

El servicio implementa arquitectura hexagonal estricta con las siguientes garantías:

- **Regla de dependencia:** dominio ← application ← infraestructura. Ningún módulo interno depende de módulos externos al hexágono.
- **Ports & Adapters:** todos los cruces de capa (DB, JWT, Google OAuth, métricas, logging) pasan por interfaces de port en dominio.
- **Multi-tenancy:** aislamiento garantizado en todos los endpoints de usuarios — validado por 8 casos e2e específicos.
- **Shared kernel:** `go-shared` (v0.1.0) provee `UserFinderService`, `MetricsRecorder`, `SecurityEventLogger` y logging estructurado como contratos entre servicios.
