# ADR-005: Tenancy — contrato S2S del read de config, fuente única de stock policy y namespace de producto

**Estado**: Aceptado para diseño (2026-10-15: revisión técnica y decisión de auditoría registradas; implementación pendiente)
**Fecha**: 2026-09-07 · **Enmendado 2026-09-15** (T1b: paso 0 de exclusión de middleware + corrección de rate-limiting) · **Enmienda aceptada 2026-10-15** (T1: bootstrap insert-only y auditoría cross-tenant; ver resolución del gate)
**Deciders**: @dev-architect, @dev-security (obligatorio — `ACC-E04` es L4), owner (sign-off, `D-12`)
**Épica**: `ACC-E04` T1 — las tres decisiones DD-1/DD-2/DD-3 que el resto de la épica ejecuta.
**Supera coordenadas de**: `ACC-E04-tenancy-absorber-tenant-config.md` §Decisiones de diseño (premisas re-verificadas hoy; dos estaban podridas — ver §Contexto, hallazgos H1 y H2).

---

## Contexto

Todas las citas de esta sección fueron re-verificadas contra el árbol vivo el **2026-09-07**
(re-`grep` del símbolo, no del número — `D-03`). Los hallazgos H1 y H2 **corrigen premisas del
archivo de la épica**: el DDL que la épica daba por pendiente ya existe, y el contrato de auth
"real" del read de config hoy no puede autenticar a sus únicos llamadores.

### Hechos verificados

| # | Hecho | Evidencia viva |
|---|-------|----------------|
| 1 | El mecanismo S2S del lab es **API key + scope** (ADR-002), con gate in-process | `src/access/infrastructure/s2s/registry.go` — `ServicePolicy` en código, secreto en `S2S_KEY_<SERVICE>`, scopes `tenant:provision`/`tenant:admin`/`system:admin`; `src/access/infrastructure/middleware/authorize.go:65` — `Authorize(jwtSecret, namespace, registry, requiredScopes, allowedRoles...)` |
| 2 | El seteo de GUC de RLS vive en la transacción del repo | `src/shared/postgres/rls.go:72` — `WithRLSInTransaction(ctx, db, tenantID, fn)`; `authorize.go:165` propaga el tenant verificado vía `sharedctx.WithTenantID` |
| 3 | El molde RLS es `019` | `migrations/019_rls_account.up.sql` — `ENABLE`+`FORCE`, policy `tenant_isolation` con `NULLIF(current_setting('app.tenant_id', true),'')::uuid`, escape `app.is_system_admin` **gated por la app tras `Authorize`**; prohibido `USING(true)` |
| 4 | Convención de grants | `migrations/018_create_app_role.up.sql:48-55` — DML a `account_app` en el DDL del rol; migraciones RLS role-agnostic; nada a `iam_login` |
| 5 | Cliente del BFF (read) | `catalog-bff-service/src/infrastructure/tenant/client/tenant_config_client.go` — `GET {baseURL}/api/v1/tenant/config/:key`, timeout 500ms, headers `X-Tenant-ID`+`X-API-Key` (si `S2S_API_KEY` está seteada), **sin `Authorization`**, `404`→`("",nil)`, `value:null`→`("",nil)`, `5xx`→error |
| 6 | El resolver lee **sólo** la key `catalog.stock_policy` | `catalog-bff-service/src/domain/tenant_stock_policy_resolver.go:87` — dominio {`REQUIRE_STOCK`,`IGNORE_STOCK`} (+`VALIDATE_STOCK` tolerado como alias, `:120`), fallback `REQUIRE_STOCK` si el destino no está configurado (`main.go:54,76-80`) |
| 7 | La columna fiscal tiene **otro** dominio | `tenant-service/src/tenant/domain/entity/tenant_settings.go:62-64` — {`IGNORE`,`RESERVE`,`DEDUCT`}; `migrations/003_create_tenant_settings.up.sql:65` — comment idem; `tenant_config` original: `migrations/001_create_tenant_config_table.up.sql:10-18`, `UNIQUE(tenant_id, config_key)` |
| 8 | Bootstrap de config es best-effort, llamado por onboarding | `onboarding-service/src/onboarding/infrastructure/client/tenant_client.go:46-75` — `POST {baseURL}/api/v1/tenant/bootstrap`, headers `X-Tenant-ID`+`X-API-Key`, **sin `Authorization`**; fallo tolerado ("best-effort, continuing") |
| 9 | Ningún otro consumidor de config en el lab | `grep -rn "tenant/config"` en `services/` → sólo el propio tenant-service y el cliente del BFF (read); nadie llama `POST /tenant/config` (set) |

### Hallazgos que corrigen la épica (re-verificación `D-03`)

**H1 — `tenants.namespace` ya existe (migración pre-ACC `010`), con la unicidad compuesta.**
La épica afirma "hoy `tenants.slug` es `UNIQUE` global" citando la migración `002`. La migración
`010_add_namespace_to_tenants.up.sql` —pre-ACC, aplicada— ya hizo el trabajo que la épica asigna a
T8: `ADD COLUMN namespace VARCHAR(50) NOT NULL DEFAULT 'mc'`, `DROP CONSTRAINT tenants_slug_key`,
`ADD CONSTRAINT tenants_namespace_slug_unique UNIQUE (namespace, slug)`, índice y backfill `'mc'`.
Verificado en vivo contra `iam_db` (`\d tenants`): la constraint compuesta existe, la
`UNIQUE(slug)` no, y `SELECT count(*) FROM tenants WHERE namespace IS NULL` → **0** (29 filas,
todas `'mc'`). **T8 no es "agregar el DDL": es normalizar lo existente** (ver DD-3).

**H2 — el read/bootstrap de config contra `tenant-service` hoy no puede autenticar.** La épica
describe el contrato actual como "deriva `tenant_id` del claim JWT". Es cierto en el código
(`tenant-service/cmd/api/main.go:36-43` — `TenantValidation` con `RejectMissingTenant: true`,
excluye sólo `/health`/`/metrics`) — pero los dos únicos llamadores de config **no mandan
`Authorization`** (hechos 5 y 8): mandan `X-API-Key`, que `TenantValidation` no consume. Tal como
está cableado, el read del BFF recibe 401 → el cliente lo trata como "unexpected status" → el
resolver loguea `stock_policy_fetch_failed` y cae al fallback `REQUIRE_STOCK`, y el bootstrap de
onboarding es best-effort y traga el fallo. Verificado en código, **no** probado en runtime: no
afirma que la policy esté caída en producción, afirma que el contrato S2S no existe —
`tenant-service` nunca tuvo superficie para llamadores S2S de config. DD-1 no es "preservar" un
contrato: es **crear** el que siempre se presupuso.

**H3 — el JWT `namespace` y `tenants.namespace` son conceptos distintos que hoy comparten valor.**
`router.go:52` — `SERVICE_NAMESPACE` default `'mc'` es el namespace del **token** (lo coteja
`Authorize` para rechazar tokens de otros proyectos, `authorize.go:120-125`); la columna de `010`
es el namespace del **producto** al que pertenece el tenant. Ambos valen `'mc'` por coincidencia
histórica. DD-3 los separa también **al nivel del valor** (ver abajo).

### Preexistencia que este ADR NO re-litiga

- **ADR-002** (S2S scoped API keys): sigue vigente. DD-1 construye sobre él; no agrega un
  mecanismo nuevo.
- **ADR-003** (RS256+JWKS, `ACC-E03` — Propuesto, `[~]` esperando firmas): explícitamente deja
  **fuera de scope** a los JWTs S2S ("ADR-002 sigue con API keys scoped"). DD-1 **no depende** del
  cutover RS256: el gate S2S de config es por API key con o sin `ACC-E03` cerrada.
- **`ACC-E02`** (aislamiento por caso de uso): roles `account_app` (NOBYPASSRLS) / `iam_login`
  (angosto, sin tenant pre-auth), patrón gated-escape de `app.is_system_admin`. `tenant_config`
  adopta exactamente ese patrón.

---

## Decisión

(1) **DD-1**: el read de config es un **S2S scoped read** — credencial del BFF con scope nuevo
`config:read` (gate `Authorize` de ADR-002) y el `tenant_id` pedido se fija como `app.tenant_id`
**sólo tras** pasar ese gate, dentro de la transacción RLS del repo (mismo patrón gated-escape que
`app.is_system_admin` de `019`, acotado al único tenant pedido). (2) **DD-2**:
`catalog.stock_policy` en `tenant_config` (mudado a `account-service`) es la **única** fuente de
verdad de vendibilidad; `tenant_settings.stock_policy` es concern fiscal AR registrado como deuda
de mercado-cercano. (3) **DD-3**: el product-namespace **ya existe** (migración `010`); se adopta
`'mercado-cercano'` como valor canónico (normalización en la migración `026`), la unicidad
`UNIQUE(namespace, slug)` queda como está, y el namespace del JWT se declara concepto distinto.

---

## Paso 0 (T4) — exclusión de las rutas de config del middleware global

> **Enmienda T1b (2026-09-15)**, originada en el gate del 2026-09-08 y persistida por el gate del
> 2026-09-15 (`management/escalations/2026-09-15_ACC-E04-T1-signoff.md`): el runtime ejecuta un
> paso previo que este ADR no conocía y que mata el contrato de DD-1 **antes** de que empiece.
> La exclusión de middleware que acá se declara es una **decisión de seguridad** (criterio (c) de
> T1): queda escrita en el diseño, no la decide ad hoc quien ejecute T4.

**Hecho verificado en disco (2026-09-15):**

- `src/router.go:53-83` monta `TenantValidation` (go-shared v0.17.0) **global**, con
  `ExcludedRoutes` en :56-65: `/health`, `/api/v1/health`, `/metrics`, `/api/v1/auth/*`,
  `/api/v1/tenants*`, `/api/v1/users*`, `/api/v1/roles*`, `/api/v1/plans*`.
- El matching de exclusiones es **por prefijo** (`go-shared/infrastructure/middleware/
  tenant_validation.go:151-162`): route con `*` → `HasPrefix(path, prefijo-trimmed)`; sin `*` →
  igualdad exacta. El prefijo `/api/v1/tenants` exige la `s`: `/api/v1/tenant/config` y
  `/api/v1/tenant/bootstrap` (singular) **no matchean ninguna exclusión**.
- **Negativa de runtime (2026-09-15, servicio vivo en :8080, ruta aún sin registrar — el
  middleware global corre también sobre paths sin route):**
  `curl -w %{http_code} http://localhost:8080/api/v1/tenant/config/catalog.stock_policy`
  → **401** `{"error":"Authorization header required"}` (`tenant_validation.go:57`), con o sin
  `X-API-Key`. El request S2S —el contrato que DD-1 crea— muere **antes** del gate `Authorize`.
- **Positiva y negativa de diseño (tests scratch contra go-shared v0.17.0 real, 2026-09-15):**
  con `"/api/v1/tenant/config*"` en `ExcludedRoutes`, el mismo request **sin** `Authorization`
  pasa el middleware y llega al handler (donde el único control es el gate `Authorize`); en
  cambio una exclusión de **match exacto** (`"/api/v1/tenant/config"`, sin wildcard) **no cubre**
  el read `/api/v1/tenant/config/:key` — `path == route` falla y el 401 sobrevive. La exclusión
  correcta es **con wildcard**.

**Decisión (paso 0 de T4):** T4 agrega a `ExcludedRoutes` (`router.go:56-65`) exactamente:

```go
"/api/v1/tenant/config*",    // GET read /:key + POST set (DD-1) — wildcard: el read lleva :key
"/api/v1/tenant/bootstrap*", // POST bootstrap S2S (DD-1)
```

con este motivo, escrito **antes** de que T4 toque código:

1. Las tres rutas de config (read, set, bootstrap) son **S2S** (o S2S+humano con rol declarado):
   su control es el gate `Authorize` (ADR-002), no la presencia de un JWT de usuario.
   `TenantValidation` exige `Authorization` en la línea :57 y responde 401 a todo request sin
   JWT — una contradicción directa con el contrato S2S, no una capa adicional de defensa.
2. La exclusión **no abre un agujero**: la superficie queda controlada fail-closed por
   `Authorize` (sin key → 401, key sin scope → 403), el guard `uuid.Nil` (→ 400), y RLS en la
   TX del repo (0 filas sin GUC). El primer control efectivo sobre la ruta pasa a ser
   `Authorize` — exactamente el mecanismo que ADR-002 diseñó para S2S.
3. Alcance cerrado: **sólo** los dos prefijos de config. Se descarta el wildcard genérico
   `/api/v1/tenant*`: excluiría en masa el espacio `tenant/` (presente y futuro) de un
   middleware de seguridad sin que cada ruta pase por revisión. Cada ruta futura bajo
   `tenant/` vuelve a requerir su propia decisión de exclusión.
4. La **escritura humano** (`POST /api/v1/tenant/config`, rol `tenant_admin`) queda excluida
   por la misma contradicción y su verificación de JWT la hace `Authorize` (firma + claims +
   rol con `allowedRoles`) — el mismo mecanismo del resto de las rutas humanas del servicio.
   No hay path sin gate: sin credencial válida, 401/403 de `Authorize`.
5. T4 re-verifica en runtime la secuencia completa: sin exclusión → 401 (ya probado acá);
   con exclusión + ruta registrada, request sin credencial → 401 **de `Authorize`**, y con
   credencial `config:read` válida → respuesta del gated-escape (criterio (b) de T1b).

---

## DD-1 · Contrato de lectura de config S2S bajo RLS

### Contexto específico

`account-service` exige el `tenant_id` de un contexto verificado (claim JWT → `app.tenant_id` →
RLS), **no** de un header crudo (`D-05`). Pero el consumidor real del read de config es el BFF del
storefront, que resuelve policy para el browse **anónimo** (hecho 6: el resolver corre por request
de catálogo, con o sin usuario logueado) y no tiene un JWT de usuario que reenviar. Además, el
contrato S2S previo nunca existió (H2): los llamadores ya hablan `X-API-Key`+`X-Tenant-ID`.

### Decisión — opción (b): S2S scoped read

Se elige la **(b)** de la épica, construida sobre ADR-002, con la firma exacta:

**Credencial y scope (extensión de `s2s/registry.go`):**

```go
// ScopeConfigRead permite leer tenant_config por key para UN tenant por request.
// No da acceso a ninguna otra tabla: la ruta sólo monta el query de config.
ScopeConfigRead Scope = "config:read"

// ServicePolicy += (política en código, secreto en env — patrón ADR-002):
"catalog-bff": {ScopeConfigRead},   // secreto: S2S_KEY_CATALOG_BFF
```

**Ruta y gate (módulo `tenancy`):**

```go
// Sólo S2S en esta épica: sin allowedRoles → el camino humano no abre en esta ruta.
configGroup := v1.Group("", scopeFactory.RequireScope(s2s.ScopeConfigRead))
configGroup.GET("/tenant/config/:key", configController.GetConfig)
```

**Secuencia obligatoria del handler — el orden ES el control (criterio (c) de T1):**

1. **Gate primero.** `Authorize` resuelve `X-API-Key` contra el registry (constant-time,
   `registry.go:113`) y exige el scope `config:read`. Sin key / key desconocida → **401**; key
   válida sin scope → **403** (`authorize.go:89-91`). El handler **no corre** si el gate no pasó:
   hasta acá el `tenant_id` pedido ni se leyó.
2. **Sólo tras el gate**, el handler lee `X-Tenant-ID`, lo parsea como UUID y **rechaza
   `uuid.Nil`** (lección `ACC-E02` T8g: un claim/header serializado en cero es un string no vacío
   parseable a Nil) → **400** si falta, es inválido o es Nil.
3. El handler propaga el tenant verificado: `sharedctx.WithTenantID(c.Request.Context(), tenantID)`.
4. El **repo** fija el GUC dentro de su transacción: `postgres.WithRLSInTransaction(ctx, db,
   tenantID, fn)` (`rls.go:72`) — `SET LOCAL app.tenant_id` vive y muere en esa TX; el aislamiento
   lo ejerce la policy de la migración `025` (molde `019`). **Prohibido** `r.db` directo dentro del
   closure (antipatrón de `tx-consistency-go`: otra conexión no ve la TX y queda fuera del
   rollback) y prohibido fijar el GUC fuera de una transacción o antes del gate.
5. Resultado: `0` filas (key inexistente **o de otro tenant** — RLS no distingue, a propósito) →
   **404**. 1 fila → **200** `{"key": "<key>", "value": "<value>"}` — misma forma que hoy espera
   el cliente (`TenantConfigResponse{Key, Value *string}`, hecho 5).

**Contrato de errores (conserva los casos del cliente, T5 no reescribe nada):**

| Estado | Causa |
|--------|-------|
| 401 | sin `X-API-Key` / key desconocida (Authorize) |
| 403 | key válida, scope insuficiente (Authorize) |
| 400 | `X-Tenant-ID` ausente, no-UUID o `uuid.Nil` |
| 404 | key inexistente para el tenant de sesión (RLS fail-closed: nunca distingue "no existe" de "es de otro") |
| 500 | error de infraestructura |

**Exposición**: in-process en `GET /api/v1/tenant/config/:key` (puerto 8080) y vía Kong bajo el
service existente `iam-service` (`/iam/` con `strip_path`, `kong.yml.template:6-12`). El plugin
`jwt` de Kong sobre ese service tiene `anonymous: "anonymous-consumer"` — el request S2S pasa el
borde como anónimo y **el gate real es el in-process** (`Authorize` existe exactamente porque el
fallback anónimo de Kong dejaba pasar requests sin token — `authorize.go:44-47`). Con o sin Kong,
el control es el mismo. El timeout agresivo (500ms), el cache con TTL y el fallback
`REQUIRE_STOCK` viven en el cliente (T5 los conserva; son su dominio, no el del servicio).

**Control de borde: rate-limiting (corrección T1b, 2026-09-15).** La versión anterior de este
ADR cerraba su lista de fuera-de-scope apoyándose en un supuesto rate-limiting service-level de
Kong sin citar plugin, config ni ubicación: una afirmación vaga e inverificable tal como estaba
escrita, que el gate marcó falsa ("el service `iam-service` no tiene el plugin"). Re-verificado hoy contra el template **y** contra el runtime
(Kong Admin `:8001`): **el control existe**, pero no donde el reviewer lo buscó — está declarado
en el array top-level de plugins con referencia al service, el mismo idioma con el que este ADR
ya describía el jwt+acl del service:

```yaml
# kong.yml.template:1290-1296 (bloque commiteado en HEAD; idéntico en runtime 2026-09-15)
- name: rate-limiting
  service: iam-service
  config:
    minute: 60
    hour: 1000
    policy: local
```

Runtime: `GET :8001/plugins?name=rate-limiting` → plugin sobre el service `iam-service`
(`a6a5adfb-…`): `minute: 60, hour: 1000, policy: local, limit_by: consumer`. La premisa del
reviewer era falsa **por dónde miró**: inspeccionó los plugins inline del bloque del service; en
Kong declarativo, un plugin top-level con `service:` **es** service-level. La afirmación vieja
queda eliminada de este ADR (verificable: cero hits del literal) y reemplazada por esta cita
verificada contra template y runtime.

**Condición de `policy: local`** (objeción 3 del gate de T1b, 2026-09-15): el contador del plugin vive
en cada nodo de Kong. Es correcto con el Kong single-node del lab; si Kong escala a más de un nodo, el límite
efectivo se multiplica por la cantidad de nodos y el plugin tiene que pasar a `policy: redis` (ya usada en
otras rutas del template) o equivalente.

Dos propiedades de ese plugin que este ADR deja explícitas:

- **Fuerza bruta contra `X-API-Key` acotada en el borde**: `limit_by: consumer` + el fallback
  `anonymous-consumer` del jwt significa que todo el tráfico S2S anónimo del service comparte
  **un** bucket de 60/min → ≤60 intentos/min de adivinar la key para TODO el tráfico anónimo.
  La key es de alta entropía y `Lookup` es constant-time (sin filtro por timing): el riesgo de
  enumeración queda despreciable.
- **Bucket compartido = 429 en tormenta**: un atacante puede quemar el bucket anónimo (60/min)
  y causar 429 al tráfico S2S legítimo — propiedad **preexistente** de todo `iam-service`
  anónimo, no introducida por la ruta de config. El cutover a un límite route-level dedicado
  (patrón de los routes catalog-flip, `kong.yml.template:634-638`) queda como decisión explícita
  del trigger de volumen en §Revisión prevista — no se adelanta acá.

Decisión: el control de borde de la ruta de config es **este** plugin service-level. **No se
agrega un plugin nuevo en esta épica**: un segundo `rate-limiting` con la misma config sobre la
ruta no cambia ninguna propiedad (el límite efectivo ya es el mínimo de los dos); si lo que se
quiere es un límite distinto (route-level con `limit_by: ip`), eso es el cutover del trigger, no
una tarea de esta épica.

**Por qué no es `USING(true)` encubierto** (criterio (c) de T1, para la revisión de seguridad): el
escape acota a **un** tenant por request, elegido por el llamador, y **sólo** después de una
decisión de autorización verificada (`Lookup` constant-time + scope). No hay enumeración: sin
conocer el UUID del tenant no hay forma de pedir filas, y la policy `tenant_isolation` de `025`
sigue comparando `tenant_id = app.tenant_id` fila a fila. El que la app pueda fijar el GUC a
cualquier tenant tras el gate es la misma propiedad que `app.is_system_admin` tiene en `019` —
deliberada, gated, con auditoría exigida por el contrato de T4 abajo (aún no implementada) —
con dos acotamientos extra que system_admin no tiene:
un solo tenant por transacción y una sola tabla (`tenant_config`) detrás del repo de config.

**Camino de escritura** (T3 muda los commands `set`/`bootstrap` con sus tests; T4 los monta sólo si
este ADR los define — acá quedan definidos):

- `POST /api/v1/tenant/config` (set de una key): **humano** JWT con rol `tenant_admin` —
  `Authorize(..., requiredScopes: [], allowedRoles: "tenant_admin")`; el tenant sale del claim
  verificado (`authorize.go:153-166`), RLS estándar, **sin escape** y sin `X-Tenant-ID` aceptado
  (el header, si viene, se ignora: el tenant es el del token). No hay consumidor S2S de escritura
  hoy (hecho 9); si aparece, exige scope propio (`config:write`) con revisión L4 aparte.
- `POST /api/v1/tenant/bootstrap`: **S2S** con scope `tenant:provision` — el mismo que ya tiene
  `onboarding` en `ServicePolicy` (hecho 1): no requiere credencial nueva, y bootstrap es
  semánticamente provision (sembrar `DefaultConfigs` para un tenant recién creado). Gate idéntico
  al del read: `X-Tenant-ID` se lee **sólo tras** el gate, se valida como UUID no nulo y se
  propaga a la TX RLS. La semántica de bootstrap es **crear sólo ausentes, nunca sobrescribir**;
  el mecanismo heredado `GetByKey` seguido de `Save`/upsert **no se conserva** (carrera descrita
  en la escalación del 2026-10-15). Rige el contrato insert-only de T3 abajo.
- **Nada a `iam_login` sobre `tenant_config`** (no hay config pre-auth; criterio (d) de T2, molde
  `019` §role-agnostic).

### Bootstrap atómico (T3) — contrato normativo

Excepción **acotada** a «cero lógica nueva» de la mudanza: corregir el check-then-upsert para
preservar la semántica existente de no sobrescribir. No cambia `DefaultConfigs`, namespaces,
scopes, datos expuestos ni el `Save` administrativo; no habilita un refactor general.

El siguiente bloque es contrato documental ejecutable, no implementación SQL/Go:

```json
{
  "operation": "bootstrap.insert_missing",
  "primitive": "INSERT INTO tenant_config",
  "conflict_clause": "ON CONFLICT (tenant_id, config_key) DO NOTHING",
  "separate_from_admin_save": true,
  "pre_read_required": false,
  "transaction": "all defaults in one WithRLSInTransaction; SET LOCAL app.tenant_id; SQL through tx only",
  "tenant_source": "validated non-nil UUID after tenant:provision gate",
  "preserve_on_conflict": ["config_value", "id", "created_at", "updated_at"],
  "created_count": "actual inserted rows after commit only; conflict=0; rollback=0 with error",
  "repeat": "existing rows unchanged; created_count=0"
}
```

T3 debe agregar una operación explícita insert-only de bootstrap, **separada** del `Save`/upsert
administrativo. El índice único resuelve la carrera incluso cuando no existe fila para bloquear;
no se sustituye por un pre-read ni se comparte la rama de actualización del admin. Los parámetros
son bind variables, el tenant es el mismo en filas y GUC y todo el lote de defaults corre en
**una TX** con el rol `account_app`, sin `r.db` dentro del closure. El contador suma sólo filas
realmente insertadas (RowsAffected/RETURNING); se publica tras commit; cualquier fallo revierte
el lote antes de commit y devuelve contador 0 + error. Si el error de commit deja resultado
ambiguo (p. ej. pérdida de conexión), se devuelve 0 + error sin afirmar ausencia durable; repetir
es seguro por idempotencia. En conflicto se preservan valor, ID y ambos timestamps
del registro existente; una posterior escritura administrativa legítima conserva su semántica.

**Aceptación T3 (DB real, pendiente):** dos conexiones independientes `account_app`, TX abiertas
y barreras/canales con timeout (no sleeps como prueba de orden). Sobre la misma `(tenant_id,key)`:
(A) admin inserta valor no default y retiene la TX; bootstrap intenta insertar; tras commit del
admin, bootstrap termina sin cambios y cuenta 0. (B) bootstrap inserta default y retiene la TX;
admin intenta su Save/upsert; tras commit de bootstrap, admin termina con su valor. Verificar
valor final admin en ambos órdenes, ID/created_at existentes intactos y updated_at no tocado por
bootstrap cuando pierde; en B el admin sí puede cambiar su updated_at. Repetir bootstrap → 0,
sin cambio de ningún campo. Forzar rollback de bootstrap tras insertar y antes de commit → 0 +
error y ninguna fila del lote persistida; liberar la TX y repetir → sólo inserciones reales.
Probar además conflicto preexistente, lote mixto (cuenta sólo ausentes), dos bootstraps concurrentes
(suma de contadores = filas nuevas), y RLS tenant A/B sin GUC residual tras commit/rollback.
**Un checker textual o mock no prueba concurrencia PostgreSQL ni reemplaza estos tests.**

### Auditoría cross-tenant (T4) — contrato normativo

Destino existente (P-21, RULE-03/RULE-08): `github.com/hornosg/go-shared/infrastructure/logging`
**v0.17.0**, `CanonicalLogger.Emit(level,event,fields)` → JSON/stdout → logs Docker con labels
`logging=promtail`, `service_name=account-service` → `observability/promtail/promtail-config.yml`
→ Loki (`http://lab-loki:3100/loki/api/v1/push`). Verificados archivos, no entrega remota.
`src/router.go` ya usa `NewSecurityLogger("iam")`, pero su payload cerrado `SecurityEvent` no
incluye `s2s_service`/outcome/correlación: no inventar campos en ese tipo ni otro logger/infra;
reusar el primitivo `CanonicalLogger` del mismo módulo, con campos tipados/allowlist en adapter
local de Infrastructure. No bajar Gin ni librerías de logging a Application/Domain.

```json
{
  "event": "tenant.config_access",
  "emitter": "tenancy Infrastructure route audit wrapper",
  "wiring": "router composition -> required canonical adapter -> wrapper before RequireScope -> controller -> final emit after outcome",
  "adapter": "go-shared v0.17.0 CanonicalLogger.Emit",
  "sink": "JSON stdout -> Docker logs -> existing Promtail -> Loki",
  "envelope": ["ts", "level", "service", "event"],
  "fields": ["s2s_service", "tenant_id", "operation", "outcome", "correlation_id"],
  "identity_source": "Authorize s2s_service context only; otherwise unattributed; never caller headers",
  "tenant_source": "validated non-nil target UUID after gate; otherwise omitted",
  "operation": ["config.read", "config.bootstrap"],
  "outcome": ["success", "not_found", "denied", "invalid_request", "error"],
  "correlation_source": "server-generated UUID; never raw caller header",
  "forbidden": ["API key", "Authorization", "config value", "raw error", "raw headers", "PII"],
  "emission": "one local attempt per completed in-process request, including gate denials and failures",
  "sink_failure": "best-effort fail-open for audit only; never bypass authorization or RLS; no retry or rollback for logging",
  "guarantee": "Emit returns void and ignores marshal/write errors; no persistence or remote delivery acknowledgement",
  "limits": "stdout may block or lose events; no durable queue, remote durability, exactly-once, or Loki delivery proof"
}
```

**Emisor/cableado exigidos, no existentes todavía:** la composición del router inyecta un adapter
canónico no opcional en el wrapper de las dos rutas S2S (read y bootstrap), colocado **antes**
de `RequireScope` para observar también `Abort`/401/403. El wrapper no autoriza ni hace otro
lookup de keys: tras `c.Next()` recoge el resultado final y el `s2s_service` que **sólo** publica
`Authorize` al pasar el gate. Un header `s2s_service`, `X-Service-Name` o equivalente no determina
identidad. Si el gate no publica identidad (incluido el 403 por scope insuficiente del gate actual),
se emite el literal `unattributed`, nunca se adivina el servicio; no se modifica auth para atribuirlo.
Sin gate/UUID válido se omite `tenant_id`, sin copiar el header crudo. Tras validarlo se toma el
mismo UUID del contexto que usa RLS: seleccionar tenant no prueba pertenencia del BFF a ese tenant.
`operation` es constante de ruta; `outcome` es enum según 200/404/401-403/400/5xx, no texto de error;
`correlation_id` es UUID generado localmente y propagado en la request. `ts` UTC y `service=iam`
son del logger (nombre actual, sin adelantar rename). Sin config key ni URL/query cruda, cuerpo,
API key, Authorization, valor de config o raw error. Éxitos `info`, denegaciones/invalid `warn`,
fallos `error`. Un fallo de DB/commit nunca se registra como éxito de bootstrap.

**Fallo del sink y límites:** `Emit` retorna void, descarta error de marshal y de `Writer.Write`;
el adapter puede asegurar payload serializable, pero no detectar ni confirmar escritura fallida.
Se intenta emitir localmente al terminar la request, sin reintentos/cola ni rollback de negocio;
la pérdida de stdout o caída de Promtail/Loki **no cambia** la decisión de auth/RLS ni el resultado
HTTP ya obtenido (fail-open sólo de auditoría). Un writer bloqueado puede bloquear la request:
no se promete latencia acotada, durabilidad remota ni entrega exactly-once. Caída del proceso,
fallo del writer o rechazo de Kong antes de entrar al wrapper pueden no dejar evento. También el gate de revocación de tokens (`token_revocation.go`), registrado sobre `apiV1` antes del wrapper, puede cortar con 401/500 sin evento (Bearer revocado, usuario inexistente o DB caída; revisión T4 2026-10-16). Guardar
`s2s_service` en Gin no es emisión ni persistencia. **Riesgo residual aceptado por el owner**
(2026-10-15, respuesta literal «usar los logs existentes» a la opción best-effort): puede haber
acceso cross-tenant sin evidencia durable. Se acepta sólo el límite del logging, no bypass de
auth/RLS. Ver `management/escalations/2026-10-15_ACC-E04-T1-decision-auditoria.md` para pregunta,
procedencia y resolución de la condición del review. No agregar outbox, BD de auditoría ni
infraestructura durable. Esta aceptación de diseño no acredita implementación ni entrega Loki.

**Aceptación T4 (adapter real, pendiente):** capturar líneas JSON del adapter real con
`NewCanonicalLoggerWithWriter("iam", writer)` y también comprobar que el composition root pasa
el adapter no opcional al wrapper y que la construcción de producción usa
`NewCanonicalLogger("iam")` (stdout, no `io.Discard`/nil). Probar 200, 404, 400, 401, 403 y fallo
DB/commit 5xx con gate real; identidad del registry al autorizar y `unattributed` al denegar,
headers de identidad falsificados sin efecto, tenant validado o ausente, operación/outcome y
correlación válidos. Introducir secretos centinela en credenciales/body/headers/errores y verificar
su ausencia en el JSON capturado. Con writer que devuelve error y writer que devuelve short-write,
el resultado auth/RLS/HTTP no cambia; no afirmar que el adapter detecta esos errores. Revisar los
labels Docker y destino configurado de Promtail. Esta captura demuestra **emisión local verificada**
y cableado; **entrega Loki no probada** salvo evidencia runtime separada. Nada de esto se ejecuta
con tests documentales de T1.

### Alternativas consideradas

- **(a) Forward del JWT del usuario** — el BFF reenvía el token del comprador: **rechazada**.
  Falla el caso que motiva el feature: el browse del storefront es anónimo (sin JWT no hay tenant
  y el resolver no puede correr). Requiriera además que el BFF atesore tokens de usuario para
  refrescar su cache en background — mayor superficie que una API key. Y el estado actual (H2)
  muestra que ningún llamador manda `Authorization` hoy: exigirlo es romper a todos los
  consumidores en vez de darles el contrato que presuponen.
- **(b) S2S scoped read con gated-escape** — **elegida**. Cubre el browse anónimo, reusa el
  mecanismo probado de ADR-002 sin superficie nueva de mecanismo, es fail-closed (sin GUC → NULL →
  0 filas) y auditable (`s2s_service` = `catalog-bff` por request). Con: introduce el primer caso
  de "tenant elegido por el llamador" en `account-service` — acotado por el gate de scope, por el
  UUID y por la única tabla; es exactamente el trade-off que la épica ya recommendó.
- **(c) Confianza por posición de red** (ruta interna sin credencial, "sólo el BFF puede llegar"):
  **rechazada**. Es la clase del god-key que ADR-002 eliminó; el lab permite pegarle al `:8080`
  sin Kong, así que "interno" no es una frontera; y el fallback anónimo de Kong ya demostró que
  "el gateway autentica" no es un control (`authorize.go:44-47`).
- **(d) mTLS/SPIFFE o JWTs firmados S2S** — **rechazada por ahora**. ADR-002 la lista como
  evolución futura y ADR-003 explícitamente deja a S2S en API keys. Agregar infra de CA interna
  para un read de config es complejidad accidental hoy; el scope `config:read` es migrable a esa
  base más adelante sin cambiar este contrato HTTP.

---

## DD-2 · Resolución de la duplicación `stock_policy`

### Contexto específico

Dos campos con el mismo nombre y distinta realidad (hechos 6 y 7): la key `catalog.stock_policy`
∈ {`REQUIRE_STOCK`, `IGNORE_STOCK`} decide **vendibilidad en el BFF** (si un variant sin stock se
muestra como comprable); la columna `tenant_settings.stock_policy` ∈ {`IGNORE`, `RESERVE`,
`DEDUCT`} decide **deducción fiscal AR al vender** (cómo el stock reacciona a la venta). Dominios
verificados en código: `tenant_stock_policy_resolver.go:64,120` y `tenant_settings.go:62-64`.
No son dos copias de un dato: son dos campos con nombre colisionante.

### Decisión

**`catalog.stock_policy` en `tenant_config` (mudado a `account-service`) es la única fuente de
verdad de vendibilidad del lab.** `tenant_settings.stock_policy` es concern fiscal AR de
mercado-cercano: queda registrado como **deuda de MC** (columna sin lector de vendibilidad —
verificado: `grep -rn "stock_policy" catalog-bff-service/src` sólo toca la key; `ACC` no la toca,
no la lee, no la renombra). T6 lo verifica con el check negativo ya pactado en la épica: ningún
path de vendibilidad lee `tenant_settings.stock_policy`.

### Alternativas consideradas

- **Unificar en un solo campo** (migrar la columna o la key al dominio del otro): **rechazada**.
  Los dominios de valores no se corresponden (`RESERVE`/`DEDUCT` no dicen nada sobre vendibilidad;
  `IGNORE_STOCK` no dice nada sobre deducción fiscal). Unificar obligaría a un mapeo con pérdida
  en alguna dirección y acoplaría al BFF con el dominio fiscal — el acoplamiento que la
  consolidación de `PLAT-PROP-013` deshace.
- **Renombrar la columna fiscal en esta épica** (`stock_policy` → p. ej. `fiscal_stock_mode`):
  **rechazada**. `ACC` no toca la tabla fiscal (`D-05`/`D-07`, y la épica lo prohíbe explícito);
  el rename toca las 307 líneas de dominio fiscal de `tenant-service`, que es trabajo de MC. Es
  la salida correcta — pero con el dueño del dato, no desde acá.
- **Declararlos distintos + fuente única + deuda registrada** — **elegida**. Cero cambio de
  esquema en MC, la duplicación *de verdad* (dos fuentes para vendibilidad) muere porque la única
  fuente pasa a ser la key, y el nombre colisionante queda documentado con motivo (`D-09`) para
  que MC decida su rename cuando toque su épica.

---

## DD-3 · Modelo de product-namespace

### Contexto específico

La épica pedía "agregar la dimensión de product-namespace" partiendo de `UNIQUE(slug)` global.
**H1 corrigió la premisa**: la migración `010` (pre-ACC, aplicada) ya agregó la columna
`namespace VARCHAR(50) NOT NULL DEFAULT 'mc'`, dropeó `tenants_slug_key` y creó
`tenants_namespace_slug_unique UNIQUE (namespace, slug)` — todo verificado en vivo contra
`iam_db` (29 filas, 0 con namespace NULL, todas `'mc'`). El DDL de la unicidad **ya está en
producción del lab**. Lo que falta no es estructura: es **(i)** el valor canónico (hoy `'mc'`,
autogenerado por un default de una migración que nadie diseñó en ACC), **(ii)** el threading en
create/get_by_slug (T9 — hoy `create_tenant.go` no setea namespace y `get_tenant_by_slug.go:20`
resuelve por `slug` solo), y **(iii)** la separación explícita del namespace del JWT (H3).

### Decisión

**(i) Valor canónico: `'mercado-cercano'`.** El valor de un namespace es el slug del producto
(`mercado-cercano`, `whatsapp-agent`, …), no una sigla. `'mc'` fue un default de `010`, no una
decisión. Elegir el nombre completo agrega una propiedad de seguridad al diseño: **diverge del
`SERVICE_NAMESPACE='mc'` del JWT** (H3) al nivel del valor, y la divergencia es la que impide que
alguien "unifique" los dos concepts por coincidencia de string. La normalización es la migración
`026` (T8 re-scoped):

```sql
-- 026_tenants_namespace_canonical.up.sql — normalización, NO creación:
-- Escape local obligatorio (enmienda T8, revisión arquitectura 2026-10-16): account_migrator es owner
-- NOBYPASSRLS y tenants tiene FORCE RLS; sin esto el UPDATE ve 0 filas y la migración estampa v26 sin
-- normalizar. Convención para toda migración de datos sobre tablas con FORCE RLS:
SELECT set_config('app.is_system_admin', 'true', true);   -- is_local=true: vive sólo en la TX del archivo
-- (la columna, el índice y UNIQUE(namespace, slug) existen desde 010)
UPDATE tenants SET namespace = 'mercado-cercano'
 WHERE namespace = 'mc';                          -- backfill del valor canónico
ALTER TABLE tenants ALTER COLUMN namespace SET DEFAULT 'mercado-cercano';
-- idempotente por ser UPDATE condicional + ALTER SET DEFAULT no-op en segundo pase;
-- down: restaurar default 'mc' y UPDATE inverso (lab: reversible, sin pérdida)
```

Se **conserva** `VARCHAR(50)` (la épica decía 100 creyendo crear la columna): el dominio son
slugs de producto del lab — `mercado-cercano` (15), `whatsapp-agent` (14) — muy lejos de 50, y
widen sin necesidad es churn gratuito. La constraint `tenants_namespace_slug_unique` **no se
toca**: ya es exactamente la objetivo. Los criterios (c)/(d) de T8 (coexistencia de un mismo slug
en dos namespaces / colisión `(namespace, slug)` → error) son **demostrables hoy** contra el
estado vivo.

**(ii) RLS no cambia.** La policy de `tenants` (`019:87-96`) sigue aislando por
`id = app.tenant_id` (+ escape `app.is_system_admin`); namespace afecta unicidad y lookup,
no aislamiento. Un tenant jamás se identifica por `(namespace, slug)` en RLS — la PK es la
identidad.

**(iii) Tres "namespace" en el lab, tres cosas distintas — prohibido unificar.**
1. `config_key` prefijo (`catalog.` en `catalog.stock_policy`): namespace **de la key** KV.
2. `tenants.namespace`: namespace **de producto** (este ADR).
3. Claim JWT `namespace` / `SERVICE_NAMESPACE` (`router.go:52`): namespace **del token/proyecto**
   — lo coteja `Authorize` para rechazar tokens de otros proyectos; unificado con `tenants.namespace`
   sólo si `ACC-E07` lo decide explícitamente al renombrar el servicio.
Con la elección de (i), 2 y 3 ya no comparten ni valor ni dueño.

### Alternativas consideradas

- **Adoptar `'mc'` como canónico** (cero churn de datos): **rechazada**. El ahorro es un
  `UPDATE` de 29 filas en un lab; el costo es quedarse con una sigla que nadie eligió y que
  **colisiona en valor** con el namespace del JWT (H3) — el mapa exacto de la confusión que esta
  decisión existe para prevenir. Además `ACC-E04`/T9 ya fijaron `'mercado-cercano'` como default
  del threading; dos valores vivos para un mismo producto romperían la unicidad por producto
  (un slug `'mc'` y un slug `'mercado-cercano'` coexistirían sin colisionar — el invariante de T8
  se evadiría sin error de constraint).
- **`UNIQUE(namespace, slug)` con namespace NULL-compatible** (namespace opcional, NULL tratado
  como un valor más, estilo "namespace default implícito"): **rechazada**. La columna ya es
  `NOT NULL` con default; NULL-compatible reintroduce el problema de "¿de qué producto es este
  tenant?" en cada query y hace el unique dependiente del tratamiento de NULLs de cada versión.
  Explícito y NOT NULL, como está.
- **Unicidad global de slug + prefijo compuesto en el slug** (`mc-foo` vs `wa-foo`):
  **rechazada**. Ensucia el slug con encoding del namespace, rompe a los consumidores existentes
  de MC y duplica en el dato la estructura que la columna ya expresa limpiamente.

---

## Consecuencias

- ✅ El BFF del storefront resuelve config con un contrato S2S **explícito** por primera vez (el
  implícito nunca autenticó — H2), con browse anónimo cubierto, sin mecanismo nuevo (ADR-002).
- ✅ `app.tenant_id` del read de config queda fijado **sólo tras** el gate de scope, dentro de la
  TX del repo: el escape es gated, acotado a un tenant y a una tabla, y falla cerrado en todos los
  estados intermedios (sin key → 401; sin scope → 403; sin header → 400; sin GUC → 0 filas).
- ✅ Una sola fuente de verdad de vendibilidad; la columna fiscal queda documentada como deuda
  de MC con su motivo, sin que `ACC` la toque.
- ✅ El DDL objetivo de unicidad ya vive en prod del lab desde `010`; `ACC-E04` lo **adopta y
  normaliza** en vez de recrearlo — menos migración, menos riesgo, mismo resultado.
- ⚠️ **T8 queda re-scoped** por H1: la migración `026` es normalización de valor + default, no
  `ADD COLUMN`/`ADD CONSTRAINT`. Quien ejecute T8 parte de este ADR, no del texto original de la
  épica (registrado en la bitácora).
- ⚠️ El read S2S acepta el tenant que el llamador nombra (por UUID): un BFF comprometido con
  UUIDs ajenos puede leer config key a key. Aceptado: es la propiedad mínima del caso de uso
  (el BFF sirve a N tenants), igual que `system:admin` acepta cross-tenant; la mitigación es el
  scope único, la una-tabla y la auditoría por `s2s_service` exigida en T4; su
  carácter best-effort y la posible pérdida de evidencia fueron aceptados explícitamente por
  el owner el 2026-10-15 («usar los logs existentes»), sin garantía de persistencia en Loki.
- ⚠️ Dos superficies de config conviven hasta que MC migre su admin (el write humano queda en
  `tenant-service` vivo): los writes a la tabla vieja no se ven en `account-service`. Sin
  consumidor de write en el lab hoy (hecho 9), el riesgo es teórico; se cierra de verdad con la
  épica de MC que retire `tenant_config` de `tenant-service`.
- 🚫 Fuera de scope: migrar a MC su admin de config; renombrar `tenant-service` (`PLAT-E34` T8);
  mTLS/JWT S2S (evolución de ADR-002); unificar namespace de token y de producto (sólo `ACC-E07`);
  rate-limiting adicional sobre la ruta de config (el control de borde service-level existe y
  está citado con evidencia en DD-1 §Control de borde; un límite route-level dedicado es el
  cutover del trigger de volumen de §Revisión prevista, no trabajo de esta épica).

## Revisión prevista

- **Si `ACC-E03` cierra y el cutover RS256 cambia el flujo de tokens**: nada de DD-1 — el gate S2S
  es por API key y ADR-003 lo deja afuera explícito. Re-visitar sólo si aparece "JWTs S2S" real.
- **Si aparece un segundo consumidor del read** (p. ej. `whatsapp-agent`, gate de `ACC-E07`):
  agregar su entrada a `ServicePolicy` con el mismo scope — el contrato no cambia; si pidiera un
  scope distinto, revisar la granularidad de `config:read`.
- **Si MC decide el rename de `tenant_settings.stock_policy`**: actualizar la referencia de deuda
  de DD-2; el check negativo de T6 se relaja recién cuando la columna deje de existir.
- **Si el volumen del read de config deja de justificar el cache del BFF** (o el TTL se vuelve un
  problema de frescura): revisar la forma del contrato (p. ej. `ETag`/`If-None-Match`), no el gate.
- **Si el volumen S2S crece** (segundo consumidor, `ACC-E07`) o el bucket compartido de 60/min
  genera 429 en producción: agregar `rate-limiting` route-level dedicado a la ruta de config
  (`limit_by: ip` o bucket propio, patrón catalog-flip) — decisión de borde en Kong, no de
  contrato. También re-evaluar entonces si el `429` merece un caso distinto en el cliente (T5
  hoy lo trata como `unexpected status`).

## Firmas (gate L4 — `ACC-E04` T1)

> Estos checkboxes los marca el gate (fase REVIEW + sign-off del owner), no el autor del ADR.
> Sin ambos marcados, T1 NO está aprobada y T2/T8 (que dependen de ella) no se ejecutan
> (`[~]` = hecho, esperando firma).

- [x] **@dev-security GO** — gate S2S de DD-1 (scope `config:read`, `app.tenant_id` fijado sólo
  tras el gate y dentro de la TX, acotamiento a una tabla, camino de escritura), DD-2 (fuente
  única) y DD-3 (valor canónico, RLS intacta, separación de namespaces).
- [x] **Owner GO** — contrato de superficie S2S nueva sobre la raíz de confianza + valor canónico
  de namespace para lo existente (`D-12`).

### Resolución documentada de la condición (2026-10-15)

El orquestador verifica satisfecha la única condición del último dictamen: aceptación expresa
de posible pérdida de auditoría. Review previo: bootstrap corregido en diseño y contrato técnico
audit favorable; su `SECURITY-REVIEW: bloqueante` histórico no se reescribe ni se inventa otro.
Registro: `management/escalations/2026-10-15_ACC-E04-T1-decision-auditoria.md`; cierre del gate
en `management/escalations/2026-10-15_ACC-E04-T1-signoff.md`. JSON normativo sin cambios.
Los GO son del diseño condicionado y su decisión registrada, no firma del owner sobre pruebas
ni autorización de publicación. Concurrencia DB y eventos del adapter real se verifican en T3/T4.
