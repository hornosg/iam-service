# ADR-003: Firma asimétrica RS256 + JWKS para tokens de usuario

**Estado**: Aprobado (gate L4 cerrado 2026-09-08 — `Owner GO` + `@dev-security GO` condicionado, condiciones cerradas por `ACC-E03` T1b; ver `management/escalations/2026-09-08_ACC-E03-T1-signoff.md`)
**Fecha**: 2026-09-07
**Deciders**: @dev-architect, @dev-security (obligatorio — `PLAT-PROP-013` §Decisiones de diseño), owner (sign-off)
**Épica**: `ACC-E03` (Firma asimétrica + JWKS) · **Corrige**: `PLAT-E14` T3 (mismo `JWT_SECRET` IAM↔Kong)

## Contexto

`iam-service` firma sus JWT de usuario con **HS256 y un secreto simétrico (`JWT_SECRET`) compartido
por toda la flota**: el mismo valor vive en el servicio, se templatiza en
`infra/api-gateway/kong.yml.template` y se inyecta por env en `tenant-service`. Con firma
simétrica, **quien puede verificar puede firmar**: cualquier componente con el secreto puede emitir
tokens válidos de cualquier otro. Para un servicio que aspira a ser cross-lab (`ACC-H0`) eso es
inaceptable — la raíz de confianza no puede repartir su capacidad de firmar.

Citas re-verificadas contra el árbol vivo el 2026-09-07 (re-`grep` del símbolo, no del número —
driftean al editar):

| Hecho | Evidencia viva |
|---|---|
| Un firmador HS256 | `src/identity/infrastructure/adapter/jwt_service.go:21` — `SigningMethodHS256` + `SignedString([]byte(s.secret))` |
| Verificador #1 (HMAC) | `jwt_service.go:27-31` (`Parse`) y `:48-52` (`ParseIgnoringExpiry`) |
| Verificador #2 (HMAC, revocación) | `src/identity/infrastructure/middleware/token_revocation.go:54` |
| Verificador #3 (HMAC, autorización) | `src/access/infrastructure/middleware/authorize.go:110` |
| Único choke point de borde | `kong.yml.template:842-845` — consumer `iam-service-consumer`, único `jwt_secrets` con `algorithm: HS256`, `secret: ${JWT_SECRET}`, `key: iam-service` |
| **20** bloques plugin `jwt` resolviendo por `key_claim_name: iss` contra esa credencial | `grep -c "name: jwt" kong.yml.template` → 20; `iss` en `:624,661,701,738,778,962,…` |
| TTL access token 15 min (ventana de cutover) | `src/identity/infrastructure/config/auth_module.go:59` (refresh 7 días, `:60`) |
| Patrón de validación de arranque a replicar | `auth_module.go:67-78` (`ValidateJWTSecret`: fatal en `GIN_MODE=release` si vacío/default/<32) |
| Refresh es edge-público (sin `jwt` en borde) | `kong.yml.template:43-58` — `iam-auth-refresh-service` con rate-limit únicamente |
| `JWT_SECRET` en wiring | `src/router.go` (`tenantmw.TenantValidation`, `jwtSecret := os.Getenv("JWT_SECRET")`) |

> Nota de drift: la épica `ACC-E03` citaba el bloque `jwt_secrets` en `:796-799`; hoy vive en
> `:842-845`. Las citas de este ADR son las re-verificadas.

## Decisión

`account-service` firma sus tokens de usuario con **RS256 (RSA ≥ 2048)**, `kid` en el header y la
clave privada PEM PKCS#8 inyectada como secreto no versionado; publica la(s) pública(s) por
**JWKS en `GET /.well-known/jwks.json`**; **Kong verifica con PEM estático por credencial**
(re-templatizado vía rebuild+recreate, sin JWKS y sin aceptación dual en el borde); el cutover usa
aceptación dual **sólo in-process**, con el flip del firmador y el de la credencial de Kong en la
misma ventana de mantenimiento y un drenaje de ≥ 15 min + skew antes de retirar HS256 (T6).

Las seis decisiones del contrato de `ACC-E03` T1:

### (a) Algoritmo — RS256

**EdDSA queda descartado.** Verificado contra el binario instalado: el plugin `jwt` de Kong OSS
3.4.0 sólo acepta credenciales con `algorithm` en
`{HS256, HS384, HS512, RS256, RS384, RS512, ES256, ES384}`
(`/usr/local/share/lua/5.1/kong/plugins/jwt/daos.lua`, `one_of`). `EdDSA` no está en la lista —
un ADR que lo eligiera haría el borde irrealizable.

**RS256 sobre ES256**, por criterio explícito:

| Criterio | RS256 | ES256 |
|---|---|---|
| Soportado por el binario instalado | ✅ (probado en vivo, ver §e) | ✅ (en el enum; no probado en vivo) |
| Tamaño de firma/token | 256 B | 64 B |
| Costo de verificación | Mayor (irrelevante al volumen del lab: tokens de 15 min, QPS de login bajo) | Menor |
| Interoperabilidad y debugging | Ubicuo: `openssl rsa`, `jwt.io`, cualquier lib cliente, `golang-jwt` | Requiere curvas correctas en cada herramienta |
| Superficie de error | "PEM RSA estándar" — un solo formato | Curvas P-256 + encoding DER específico; errores sutiles de formato |

El tamaño y el costo de verificación son ventajas que el lab no necesita; la interoperabilidad es
la que sí necesita (el JWKS lo consumen herramientas Go, agentes y frontends ajenos a este repo).
**La complejidad accidental gana el argumento**: RS256 es el path con menos formas de romperse.

### (b) Clave privada — PEM PKCS#8, secreto no versionado, validación de arranque

- **Formato**: RSA ≥ 2048 en PEM **PKCS#8** (cabecera `BEGIN` de clave privada). PKCS#8 sobre PKCS#1 porque es
  el formato que `x509.ParsePKCS8PrivateKey` y la mayoría del tooling esperan por default.
- **Inyección**: `JWT_PRIVATE_KEY_FILE` (path a archivo montado) es el mecanismo **canónico** —
  un PEM multilinea en una env var rompe el tooling de `.env` de docker-compose y el
  `entrypoint.sh` de Kong. `JWT_PRIVATE_KEY` (inline, env) se acepta **sólo para desarrollo
  local** (`go run`). Ambas presentes → error fatal; ninguna → error fatal. En k3s (`PLAT-E14`)
  el archivo llega montado desde un Secret de Kubernetes, mismo nombre de var.
- **Validación de arranque** análoga a `ValidateJWTSecret` (`auth_module.go:67-78`): leer, parsear
  como PKCS#8, verificar que sea `*rsa.PrivateKey` y que el módulo sea ≥ 2048; `log.Fatalf` en
  `GIN_MODE=release` si falla cualquiera, warning en dev.
- **Ningún byte privado se versiona** — lo verifica la tarea T2 (`git ls-files`, `git log -p`,
  `git check-ignore`).
- El `kid` se registra al generar la clave (T2) y se fija en config junto al par.

### (c) Esquema `kid` + política de rotación con solapamiento

- **Forma del `kid`**: **thumbprint determinístico de la clave pública** — SHA-256 sobre la
  representación JWK canónica (estilo RFC 7638), primeros **12 hex**, prefijado: `acc-<12hex>`.
  Racional: el `kid` queda **ligado al material de la clave** (dos claves distintas nunca
  comparten `kid` por accidente de numeración) y no requiere coordinación manual; con un contador
  (`v1`, `v2`) un re-generado descartado dejaría huecos y ambigüedad de "¿cuál es v2?".
- **El `kid` viaja en el header** de todo token firmado y en cada entrada del JWKS.
- **Rotación (publicar → firmar → gracia → retirar)**:
  1. **Publicar**: agregar la pública nueva al JWKS **antes** de firmar con ella.
  2. **Firmar**: el firmador cambia al `kid` nuevo una vez publicado.
  3. **Gracia**: los verificadores in-process aceptan ambos `kid` durante ≥ TTL del access token
     (15 min, `auth_module.go:59`) + skew (default NTP del lab) — un token firmado con la clave
     vieja sigue validando hasta su expiración natural.
  4. **Retirar**: sacar la clave vieja del JWKS y del keyring de los verificadores; desde ese
     momento los tokens `kid` viejo se rechazan.
- **Esperas de propagación (ACC-E03 T8, condición C3 del veredicto de T7).** El JWKS se sirve con
  `Cache-Control: public, max-age=300`. Por eso:
  - **Entre publicar y firmar: ≥ 300 s.** Un consumidor JWKS que cacheó el documento justo antes de
    publicar no conoce `kid2` hasta que su copia expira; firmar antes deja sus tokens `kid2` en 401
    en ese consumidor durante hasta 5 minutos. Los verificadores de keyring montado (iam y la flota
    go-shared) no cachean: ven `kid2` al recrearse.
  - **Cola de revocación de 5 minutos.** Ante una clave comprometida, retirarla del keyring corta en
    iam y en la flota al recrear, pero un consumidor JWKS con el documento cacheado puede seguir
    aceptándola **hasta 5 minutos** más. Es el límite superior de exposición post-retiro para esos
    consumidores; si alguno lo necesita más corto, baja el `max-age`, no se saltea el retiro.
- **Mecánica en iam (T8, condiciones C1/C2).** El verificador de iam carga, además de la pública de
  firma, las públicas de `JWT_PUBLIC_KEY_DIR` (el mismo keyring montado de la flota, vía
  `jwtkeyring.Load` de go-shared: sólo `PUBLIC KEY` RSA ≥2048, kid derivado del material). Es una
  sola keyring, inmutable tras el arranque, compartida por `Parse`, `token_revocation`, `authorize`
  y el JWKS — lo publicado es lo aceptado. Retirar = sacar el `.pem` del keyring y recrear iam **y
  la flota** (condición C4). Simulacro: `test/identity/auth/rotation/rotation_t8_test.go` +
  bitácora de ACC-E03 (2026-10-17).
- **Escribir en el keyring equivale a poseer la privada de firma (T8, condición C-T8-4 del gate L4).**
  Desde T8 cualquier `.pem` público del directorio también es aceptado por iam, incluido su plano de
  administración (`/users`, `/tenants`, `/roles`, `/plans`) con `system_admin`, no sólo por la flota.
  Requisito de despliegue en producción (k3s): el ConfigMap/Secret del keyring debe tener un RBAC de
  escritura tan restrictivo como el Secret de la clave privada. En el lab el directorio es del uid del
  host, que ya lee la privada, por lo que no hay escalada.
- Kong consume PEM estático por credencial (§e): en una rotación, la credencial de
  `iam-service-consumer` se re-templatiza con la pública nueva y se hace rebuild+recreate en el
  mismo paso "publicar→firmar". **No hay dual de borde en rotación** (misma restricción que el
  cutover, §e: `cache_key = { key }` es unique — no pueden coexistir dos credenciales con el
  mismo `key: iam-service` y distinto algoritmo).
- **Costo real de rotar sin dual de borde (declarado, no asumido).** El flip de credencial y de
  firmador en el mismo deploy sólo cubre los tokens **nuevos**: los tokens `kid1` **no
  expirados** emitidos antes del flip dan **401 en el borde** al instante (Kong tiene una sola
  pública, la nueva), igual que en el cutover de §f. El golpe es el mismo 401→refresh ya
  aceptado: el cliente pega al refresh edge-público (token opaco, lookup en DB) y recibe un
  token `kid2` sin re-login. No es un hueco de seguridad — es el costo de no tener dual de
  borde, y hay que declararlo para que T8 no simule una rotación cuyo costo real nadie escribió.
- **Consecuencia para T8**: el criterio (a) de T8 ("token `kid1` → 200 durante el solapamiento")
  **sólo es realizable in-process o contra consumidores JWKS**, nunca contra un endpoint gateado
  por Kong (que ya flippeó a la pública nueva). T8(a) queda **re-alcanceado a verificación
  in-process**; el borde en rotación se comporta como el cutover (401→refresh), no como
  solapamiento.
- El simulacro de T8 ejercita este ciclo completo contra un entorno descartable.

### (d) Forma del JWKS

- **Ruta**: `GET /.well-known/jwks.json` (convención OIDC; RFC 7517 no fija path). **Pública, sin
  auth** — el JWKS es material público por definición (verificar material privado adentro es el
  criterio (c) de T7).
- **JSON**:

```json
{
  "keys": [
    {
      "kty": "RSA",
      "use": "sig",
      "alg": "RS256",
      "kid": "acc-1a2b3c4d5e6f",
      "n": "<módulo, base64url>",
      "e": "AQAB"
    }
  ]
}
```

- `n`/`e` son el material público RSA; **nunca** `d`/`p`/`q` (su presencia es fuga de la privada).
- Durante la gracia de una rotación, el array incluye la clave vieja y la nueva.
- **Consumidores**: servicios Go del lab, `whatsapp-agent`, `web-display` y cualquier integración
  futura que verifique tokens in-process. **Kong NO está en esta lista** (§e).

### (e) Cómo verifica Kong — PEM estático por credencial, verificado contra el binario

**Verificación contra el binario instalado (no asumida), 2026-09-07:**

```
$ docker exec lab-kong kong version
3.4.0
```

1. **No hay auto-descarga de JWKS.** El schema de la credencial `jwt_secrets` del plugin
   (`/usr/local/share/lua/5.1/kong/plugins/jwt/daos.lua`) tiene exactamente los campos
   `id, created_at, consumer, key, secret, rsa_public_key, algorithm, tags` — **ningún campo URL /
   JWKS / discovery**. El handler resuelve la clave de verificación como
   `jwt_secret.secret or jwt_secret.rsa_public_key` (`handler.lua:203`): PEM estático o secreto
   HMAC, nada más.
2. **RS256 requiere PEM válido en la credencial.** `daos.lua` define
   `one_of = {HS256, HS384, HS512, RS256, RS384, RS512, ES256, ES384}` con un `entity_check`
   condicional: `RS256/384/512` → `rsa_public_key` **required** con validador `openssl_pkey`
   (rechaza PEM inválido). (`ES256` usa el mismo campo con un PEM EC.)
3. **Prueba viva** — Kong 3.4.0 DB-less descartable en `lab-network` con credencial
   `algorithm: RS256` + `rsa_public_key` (PEM), route con plugin `jwt`:

   | Request | Resultado |
   |---|---|
   | Bearer RS256 firmado con la privada correspondiente | **200** (la autenticación pasó; alcanzó el upstream) |
   | Bearer `alg:none` | **401** |
   | Bearer RS256 firmado con OTRA clave privada | **401** |
   | Bearer HS256 "firmado" usando el PEM público como secreto HMAC (confusión RS/HS) | **401** |
   | Sin token | **401** |

**Conclusión**: Kong OSS/community 3.4.0 verifica RS256 con **PEM estático en la credencial**.
El diseño de esta épica es realizable tal como está: T5 re-templatiza la credencial con
`rsa_public_key: ${JWT_PUBLIC_KEY}` y aplica **rebuild + recreate**
(`docker compose -f infra/docker-compose.yml up -d --build kong` — el `restart` reusa la imagen
con el template viejo, verificado 2026-07-09).

**Consecuencia del "no-dual" en el borde.** La credencial se resuelve por `key_claim_name`
(default `iss` en los 20 bloques) y `cache_key = { key }` es **unique**: **no pueden coexistir dos
credenciales con el mismo `key: iam-service` y distinto algoritmo**. Si se quisiera aceptación
dual en el borde habría que mover `key_claim_name` de `iss` a `kid` en **los 20 bloques** del
template — blast radius: 20 edit points, y **aún así no cubriría a los tokens HS256 ya emitidos**,
que no llevan `kid` en header ni en payload → fallarían el lookup igual. El dual de borde es
irrealizable para los tokens vivos **por diseño del plugin**, no por decisión nuestra.
Se rechaza y el cutover del borde es un **flip único de credencial** (§f).

### (f) Estrategia de cutover dual — anclada al TTL de 15 min

**Regla de selección dual (in-process, durante la ventana).** Con dos métodos aceptados, el
verificador elige por el header del token, nunca por intento y error:

| `alg` del header | Clave de verificación |
|---|---|
| `HS256` | **sólo** el secreto viejo (`JWT_SECRET`) |
| `RS256` | la pública por `kid` (keyring del JWKS) |
| `none` / cualquier otro | **rechazo** |

**Jamás** se usa la pública RSA como secreto HMAC (confusión RS/HS): un token `HS256` firmado
con la pública tratada como secreto debe fallar. Los tests negativos de confusión ya están
exigidos en T3/T4; esta regla es la especificación que esos tests verifican, no redundancia.

1. **Preparación** (T2–T4): par de claves generado, firmador RS256+kid (`Sign`), y los tres
   verificadores in-process con **aceptación dual** (HS256 con el secreto viejo Y RS256 por
   `kid`) durante la ventana.
2. **Flip** (T5 + deploy del firmador, **misma ventana de mantenimiento**): el login pasa a
   emitir RS256 **y** la credencial de Kong se flippea a RS256 en el mismo deploy. No hay orden
   seguro que permita "después": si Kong flippea antes, los tokens HS256 nuevos dan 401 en el
   borde; si flippea después, los RS256 nuevos dan 401. Un solo cambio atómico.
3. **Drenaje**: los access tokens HS256 vivos al momento del flip expiran en ≤ 15 min
   (`auth_module.go:59`). El golpe para un usuario activo con token HS256 es **transparente**:
   sus requests gateados por Kong dan 401, el cliente pega al refresh —
   `/iam/api/v1/auth/refresh` es **edge-público** (service dedicado sin `jwt`,
   `kong.yml.template:43-58`) y el refresh token es **opaco** (se resuelve por lookup en DB, no
   por firma) → recibe un access token RS256 y sigue sin re-login. Los clientes sin lógica de
   refresh re-loguean; costo acotado al lab.
4. **Retiro** (T6): tras ≥ 15 min + skew desde el flip, se retira la aceptación dual de los tres
   verificadores in-process y `JWT_SECRET` deja de firmar/verificar tokens de usuario
   (auditoría de usos residuales incluida — `tenant-service` se audita con grep, no por memoria
   del roadmap). `PLAT-E14` T3 queda **anulada** con puntero a esta épica.

La aceptación dual in-process no es redundante aunque el borde ya flippeó: cubre el acceso
directo a los endpoints gateados in-process durante la ventana (el lab permite pegarle al
`:8080` sin pasar por Kong) y es margen de seguridad ante un rollback parcial del deploy del
firmador.

## Alternativas consideradas

- **RS256 (elegida)**: interoperable, verificada en vivo contra el binario instalado, un solo
  formato de PEM. Con: firmas de 256 B y verificación RSA — costo irrelevante al volumen del lab.
- **ES256**: firmas de 64 B y verificación más barata, soportado por el enum del plugin. Con:
  requiere PEM EC en `rsa_public_key` (nombre engañoso), curvas correctas en cada herramienta de
  debugging, y no aporta nada que el lab necesite. Rechazada por complejidad accidental.
- **EdDSA**: descartada — no está en el `one_of` del plugin `jwt` de Kong OSS (verificado contra
  `daos.lua`). El borde no podría verificarla.
- **Rotar `JWT_SECRET` y seguir en HS256**: no cambia la propiedad inaceptable — verificar y
  firmar siguen siendo la misma capacidad repartida por la flota. Rechazada (ver Notas de la
  épica).
- **Dual en el borde vía `key_claim_name: kid` en los 20 bloques**: rechazada — irrealizable para
  los tokens HS256 vivos (no llevan `kid`) y toca 20 puntos del template por cero cobertura
  extra. El cutover atómico de §f la vuelve innecesaria.

## Consecuencias

- ✅ Verificar tokens (pública, repartible) y firmarlos (privada, exclusiva de account-service)
  pasan a ser **capacidades separadas** — condición de cierre de `ACC-H0`.
- ✅ La fuga de la pública deja de ser incidente: es material público por diseño (JWKS).
- ✅ Kong migra de una sola vez: un único choke point de credencial (`kong.yml.template:842-845`)
  sirve a los 20 bloques.
- ⚠️ Aparece gestión de material criptográfico (T2) y una nueva superficie pública (JWKS, T7)
  que hay que auditar por fuga de material privado.
- ⚠️ La rotación exige un ciclo de solapamiento ejecutable (T8 lo demuestra); sin él, la
  política es decoración.
- ⚠️ El cutover transitorio mantiene 15 min de aceptación dual in-process: es ventana de
  ataque conocida y acotada, retirada en T6.
- 🚫 Fuera de scope: JWTs S2S (ADR-002 sigue con API keys scoped), mTLS/SPIFFE (evolución futura
  de ADR-002), rotación automática sin intervención (el simulacro T8 es manual), firma de tokens
  de servicio/`tenant-service` (sólo se audita que no firma/verifica tokens de usuario).

## Revisión previa

- **Cambio de versión mayor de Kong** o reemplazo del plugin `jwt`: re-verificar §e contra el
  nuevo binario (el enum de algoritmos y la ausencia de JWKS son propiedades del binario, no del
  diseño).
- **Materialización de `PLAT-E14` (k3s)**: revisar cómo se monta `JWT_PRIVATE_KEY_FILE` y cómo se
  re-templatiza la pública en el gateway de prod; `PLAT-E14` T3 está anulada por esta épica y no
  puede reintroducir `JWT_SECRET` compartido.
- **Volumen de verificación** que haga relevante el costo de RSA (miles de QPS): re-evaluar ES256
  — los consumidores ya resuelven por `kid`/JWKS, el cambio sería local.
- **Integración OIDC real** (issuer/discovery completos): el JWKS de (d) es el punto de partida;
  no se vuelve a decidir el algoritmo.

## Addendum — Verificadores de flota (`ACC-E03` T5a) — rev. 2 (2026-09-15)

> Escrito por la iteración EXEC de `ACC-E03` T5a a partir del hallazgo de la verificación del gate
> de T4 (bitácora, 2026-09-14): existe un **cuarto verificador HMAC que este ADR no inventarió**, y
> es de **flota**, no de `iam-service`. **Rev. 2**: la rev. 1 fue **RECHAZADA por el gate L4**
> (2026-09-15, `escalations/2026-09-15_ACC-E03-T5a-signoff.md`) porque su criterio de inventario
> sólo buscaba `TenantValidation(` y perdió a los verificadores que parsean el JWT por su cuenta y
> a los emisores de service tokens. Esta rev. re-hace el inventario con el grep de **5 patrones**
> (§B), cierra la mecánica de rotación de la clave pública en la flota (§C), inventaria los
> emisores HS256 (§D) y re-alcancea la secuencia (§E). Las decisiones quedan **condicionadas a las
> firmas de su gate** (abajo); hasta entonces T5 no puede flippear el borde.

### A. Los verificadores de flota — lo que este ADR no inventarió (y la rev. 1 tampoco)

Cuatro verificadores HMAC **fuera de iam-service** verifican tokens de usuario (o tokens firmados
con el mismo secreto) en proceso:

1. **go-shared `TenantValidation`** (`active/mercado-cercano/libs/go-shared/infrastructure/middleware/tenant_validation.go`,
   v0.17.0) — keyfunc clavada a la **familia HMAC completa** (`tenant_validation.go:69` —
   `*jwt.SigningMethodHMAC`, o sea HS256/HS384/HS512, no sólo HS256) contra `cfg.JWTSecret`, y
   `401 Abort` ante cualquier error. Es la fuente canónica del inventario de `PLAT-E46` (fail-closed
   `JWT_SECRET` en la flota).
2. **go-shared `VerifyPlatformJWT`** (`platform_auth.go:63`) — keyfunc HMAC contra `cfg.JWTSecret`
   con `WithExpirationRequired` (gate L4 de PLAT-E39, 2026-07-28: sin RLS debajo, el middleware es
   el control único de las tablas de plataforma). Hoje lo monta **catalog-service**.
3. **`AuthMiddleware` propio de webdata-service** (`src/shared/middleware/auth_middleware.go:71`,
   http.Handler, montado en `src/api/router.go:40`) — keyfunc HMAC contra
   `os.Getenv("JWT_SECRET")` con `WithExpirationRequired`; fail-closed, y es el control único de
   sus rutas de admin (el rol viene del claim `roles` del JWT ya verificado, D3.1). La rev. 1
   afirmaba que webdata verificaba "sólo en el borde": **falso** — hallazgo #1 del rechazo.
4. **`VerifyTenantCommandJWT` propio de catalog-service**
   (`src/globalproductrequest/infrastructure/middleware/verify_tenant_command_jwt.go:50`, montado en
   `cmd/api/main.go:325`) — keyfunc HMAC contra `cfg.JWTSecret`, con `ExpectedIssuer: "iam-service"`
   y `namespace: mc`. **No verifica tokens de servicio: verifica los tokens de USUARIO de iam
   reenviados por PIM** en el flujo de global product requests (PLAT-E39 D2: "PIM verifica el JWT
   original ... y reenvía ESE MISMO JWT"). Post-flip de T5, iam emite RS256 → los tokens reenviados
   serán RS256 → este verificador HMAC-only los rechazaría. El dual de este verificador es
   **prerequisito del flip**, no deuda de T6 (ver §E); su **retiro** con destino explícito sí es de
   T6 (hallazgo #2 del rechazo).

Además de verificar la firma, `TenantValidation` **propaga identidad al contexto**
(`tenant_validation.go:100-107`): `tenant_id`, `jwt_claims`, `roles`, `namespace` — y aplica el gate
de mismatch (`X-Tenant-ID` vs `tenant_id` → 403), el gate de `namespace` y `RejectMissingTenant`
(403 ante token sin `tenant_id`; hoy activo en los call sites del inventario §B). `RequireRole` de
go-shared **depende** de ese contexto (`require_role.go:11-12`). No es un re-verificador redundante:
es el gate de tenancy y la fuente de claims de los gates de rol.

**El borde no lo reemplaza.** Los 14 bloques `jwt` de Kong que gatean estos servicios llevan
`anonymous: "anonymous-consumer"` (`kong.yml.template`, bloques de plugin con `service:` —
iam/pim/stock/catalog-bff/sales/customer/ledger/payment-method/marketplace/onboarding/ai-gateway/webdata/notification/tenant):
con `anonymous` configurado el plugin **no** rechaza requests sin Bearer — las deja pasar al
upstream. El 401 "Authorization header required" (`tenant_validation.go:56-58`) es lo que hace el
borde efectivo para requests sin token. **Un consumidor que deje de verificar in-process sin
retirar el `anonymous` queda con acceso sin credenciales a sus rutas gateadas.**

`go-shared` (`github.com/hornosg/go-shared` `infrastructure/middleware/tenant_validation.go`,
fuente en `active/mercado-cercano/libs/go-shared/`) parsea el Bearer con una keyfunc clavada a la
**familia HMAC completa** (`tenant_validation.go:68-69` — `*jwt.SigningMethodHMAC`, o sea
HS256/HS384/HS512, no sólo HS256) contra `cfg.JWTSecret`, y hace `401 Abort` ante cualquier error.
Es la fuente canónica del inventario de `PLAT-E46` (fail-closed `JWT_SECRET` en la flota).

Además de verificar la firma, el middleware **propaga identidad al contexto** (`tenant_validation.go:100-107`):
`tenant_id`, `jwt_claims`, `roles`, `namespace` — y aplica el gate de mismatch
(`X-Tenant-ID` vs `tenant_id` → 403), el gate de `namespace` y `RejectMissingTenant` (403 ante token
sin `tenant_id`; hoy activo en 9 de los 11 call sites). `RequireRole` de go-shared **depende** de
ese contexto (`require_role.go:11-12`: "Debe ejecutarse DESPUÉS de TenantValidation, que es quien
deja `jwt_claims` en el contexto"; `:67-71`). No es un re-verificador redundante: es el gate de
tenancy y la fuente de claims de los gates de rol.

**El borde no lo reemplaza.** Los 14 bloques `jwt` de Kong que gatean estos servicios llevan
`anonymous: "anonymous-consumer"` (`kong.yml.template`, bloques de plugin con `service:` —
iam/pim/stock/catalog-bff/sales/customer/ledger/payment-method/marketplace/onboarding/ai-gateway/webdata/notification/tenant):
con `anonymous` configurado el plugin **no** rechaza requests sin Bearer — las deja pasar al
upstream. El 401 "Authorization header required" (`tenant_validation.go:56-58`) es lo que hace el
borde efectivo para requests sin token. **Un consumidor que deje de verificar in-process sin
retirar el `anonymous` queda con acceso sin credenciales a sus rutas gateadas.**

### B. Inventario canónico — el grep de 5 patrones, línea por línea (corrido 2026-09-15 desde `$DEVY_PATH`)

El criterio (a) corregido por el rechazo del gate: el inventario ya no son sólo los call sites de
`TenantValidation(` — es **cada línea** que devuelve el grep ampliado:

```bash
grep -rnE --include='*.go' --exclude-dir=node_modules --exclude-dir=vendor \
  'TenantValidation\(|VerifyPlatformJWT\(|VerifyTenantCommandJWT\(|SigningMethodHMAC|SigningMethodHS256' \
  active platform pocs \
  | grep -v _test.go \
  | grep -vE 'func (TenantValidation|VerifyPlatformJWT|VerifyTenantCommandJWT)\('
```

→ **25 líneas** (2026-09-15). Clasificación de CADA una: verificador de flota / emisor HS256 /
verificador propio de iam / definición en go-shared.

| # | Servicio (repo) | Call site | Clasificación | Alcance |
|---|---|---|---|---|
| 1 | go-shared | `infrastructure/middleware/tenant_validation.go:69` | Definición en go-shared | keyFunc de `TenantValidation` — familia HMAC completa, `cfg.JWTSecret` |
| 2 | go-shared | `infrastructure/middleware/platform_auth.go:63` | Definición en go-shared | keyFunc de `VerifyPlatformJWT` — HMAC, `cfg.JWTSecret`, `WithExpirationRequired` |
| 3 | customer-service | `src/main.go:52` | Verificador de flota (`TenantValidation`) | rutas de usuario detrás — §B.2 fila 1 |
| 4 | catalog-bff-service | `main.go:26` | Verificador de flota (`TenantValidation`) | rutas de usuario detrás — §B.2 fila 2 |
| 5 | sales-service | `main.go:82` | Verificador de flota (`TenantValidation`) | rutas de usuario, **maneja dinero** — §B.2 fila 3 |
| 6 | payment-method-service | `src/main.go:41` | Verificador de flota (`TenantValidation`) | rutas de usuario — §B.2 fila 4 |
| 7 | stock-service | `main.go:68` | Verificador de flota (`TenantValidation`) | rutas de usuario — §B.2 fila 5 |
| 8 | tenant-service | `cmd/api/main.go:36` | Verificador de flota (`TenantValidation`) | montaje **global** — §B.2 fila 6 |
| 9 | pim-service | `main.go:65` | Verificador de flota (`TenantValidation`) | 0 rutas anónimas — §B.2 fila 7 |
| 10 | ledger-service | `cmd/server/main.go:119` | Verificador de flota (`TenantValidation`) | balance financiero — §B.2 fila 8 |
| 11 | onboarding-service | `src/main.go:56` | Verificador de flota (`TenantValidation`) | wizard público excluido — §B.2 fila 9 |
| 12 | notifications | `src/main.go:45` | Verificador de flota (`TenantValidation`) | grupo `/api/v1` — §B.2 fila 10 |
| 13 | iam-service | `src/router.go:53` | Verificador de flota — **inerte** | todas sus rutas caen en excluidas (golden de rutas, T4) — §B.2 fila 11 |
| 14 | support-service (pocs) | `src/main.go:44` | Verificador de flota en **PoC** | `pocs/riotless` — exclusión explícita con justificación: §D |
| 15 | webdata-service | `src/shared/middleware/auth_middleware.go:71` | Verificador propio de flota | `AuthMiddleware` — tokens de USUARIO, control único de admin (D3.2) |
| 16 | catalog-service | `cmd/api/main.go:162` | Verificador de flota (go-shared) | monta `VerifyPlatformJWT` + `RequireRole("system_admin")` — `platformGroup` |
| 17 | catalog-service | `cmd/api/main.go:325` | Verificador de flota | monta `VerifyTenantCommandJWT` — grupo `/api/v1/s2s`, tokens de usuario reenviados por PIM |
| 18 | catalog-service | `src/globalproductrequest/infrastructure/middleware/verify_tenant_command_jwt.go:50` | Definición de verificador (en catalog, no go-shared) | keyFunc HMAC de `VerifyTenantCommandJWT` — `ExpectedIssuer: iam-service` |
| 19 | webdata-service | `src/shared/platformauth/service_token.go:94` | **Emisor HS256** | service token hacia catalog (`roles: system_admin`, `iss: webdata-service`); secret `os.Getenv("JWT_SECRET")` (`:46`); lo verifica catalog con `VerifyPlatformJWT` |
| 20 | onboarding-service | `src/onboarding/infrastructure/auth/service_token_provider.go:112` | **Emisor HS256** | service token hacia iam (`iss: iam-service`, tenant system, namespace) vía `lab-kong:8000/iam-service/api/v1/tenants*`; secret `JWT_SECRET` (`setup.go:34`) |
| 21 | iam-service | `scripts/generate-onboarding-token.go:49` | **Emisor HS256** (tooling) | script CLI **DEPRECATED** — fallback del flujo de onboarding; no corre en runtime |
| 22 | iam-service | `src/access/infrastructure/middleware/authorize.go:30` | Verificador propio de iam | `dualAcceptedMethods` {HS256, RS256} — post-T4, en alcance aprobado |
| 23 | iam-service | `src/access/infrastructure/middleware/authorize.go:142` | Verificador propio de iam | rama HS256→secreto del gate `authorize` (T4) |
| 24 | iam-service | `src/identity/infrastructure/adapter/jwt_service.go:80` | Verificador propio de iam | rama HS256→secreto del `Parse` dual (T3) |
| 25 | iam-service | `src/identity/infrastructure/adapter/jwt_service.go:130` | Verificador propio de iam | `dualAcceptedMethods` del firmador (T3) |

`token_revocation.go` **no aparece** en el grep: desde T4 delega el parse al verificador dual del
firmador (`cfg.JWT.Parse`, `TokenRevocationConfig.JWT`) y ya no menciona ningún método por su
cuenta. No hay líneas sin clasificar: 25/25 en la tabla. (El test de contrato
`test/adr/adr003_t5a_test.go` ejecuta ESTE mismo grep y falla si alguna línea queda fuera.)

#### B.2 Detalle de los call sites `TenantValidation` — rutas excluidas y fail-closed

| # | Servicio | Call site | Rutas excluidas | ¿Rutas de usuario detrás? | `RejectMissingTenant` | `Namespace` |
|---|---|---|---|---|---|---|
| 1 | customer-service | `src/main.go:52` | `/health`, `/api/v1/health`, `/metrics` | Sí — todo el resto | true | — |
| 2 | catalog-bff-service | `main.go:26` | `/health`, `/api/v1/health`, `/metrics`, `/api/v1/marketplace/categories`, `.../tree` | Sí — resto de marketplace/products/stores (PLAT-E39 T8c2 cerró el wildcard anónimo) | true | — |
| 3 | sales-service | `main.go:82` | `/health`, `/api/v1/health`, `/metrics` | Sí — caja/POS, **maneja dinero** | true | `mc` |
| 4 | payment-method-service | `src/main.go:41` | `/health`, `/api/v1/health`, `/metrics` | Sí | true | — |
| 5 | stock-service | `main.go:68` | `/health`, `/metrics` | Sí | true | — |
| 6 | tenant-service | `cmd/api/main.go:36` | `/health`, `/metrics` | Sí — **global**, casi todas sus rutas | true | — |
| 7 | pim-service | `main.go:65` | `/api/v1/health`, `/metrics` | Sí — 0 rutas anónimas (PLAT-E39 T14) | **false** (único sin fail-closed de tenant) | — |
| 8 | ledger-service | `cmd/server/main.go:119` | `/health`, `/metrics` | Sí — balance financiero | true | — |
| 9 | onboarding-service | `src/main.go:56` | `/health`, `/api/v1/health`, `/metrics`, `/api/v1/onboarding/*` (wizard público) | Sí — el resto | true | — |
| 10 | notifications | `src/main.go:45` | `/health`, `/metrics` | Sí — `api := r.Group("/api/v1")` | `multitenant` (condicional, fail-closed) | — |
| 11 | iam-service | `src/router.go:53` | `/health`, `/api/v1/health`, `/metrics`, `/api/v1/auth/*`, `/api/v1/tenants*`, `/api/v1/users*`, `/api/v1/roles*`, `/api/v1/plans*` | **Inerte** — el golden de rutas confirma que todas las rutas caen en excluidas (verificado T4, bitácora) | (hereda config propia) | `mc` |
| 12 | support-service (pocs) | `src/main.go:44` | `/health`, `/metrics` | PoC, sin tráfico de usuario del lab — **exclusión explícita en §D** | `multitenant` (condicional) | — |

Los 11 call sites de `active/` consumen `go-shared v0.17.0` (verificado en cada `go.mod` el
2026-09-14) — un release de go-shared con dual + un bump homogéneo cubre la flota de una vez.
**Corrección de la rev. 1**: la afirmación "marketplace-service, ai-gateway-service y
webdata-service verifican sólo en el borde" era **falsa para webdata** (verificador #15) y sin
evidencia para los otros dos. Evidencia de esta rev.: ni `marketplace-service` ni
`ai-gateway-service` devuelven línea en el grep (sin `jwt.Parse`/`SigningMethod` en su código,
verificado 2026-09-15) — para ellos el flip de Kong sí es transparente. La lista de los 14 bloques
`anonymous` no cambia.

#### B.3 Delta de T5b (2026-10-16) — reclasificación tras la implementación dual

Tras `ACC-E03` T5b (go-shared v0.18.0) el grep canónico devuelve 24 líneas. La tabla firmada de §B
no se edita; este delta clasifica lo que cambió de lugar o de naturaleza:

| Servicio | `archivo:línea` | Clasificación |
|---|---|---|
| go-shared | `infrastructure/jwtkeyring/jwtkeyring.go:40` | definición de `AcceptedMethods` (HS256 fijado + RS256) del keyFunc dual — no es un call site de verificación |
| go-shared | `infrastructure/jwtkeyring/jwtkeyring.go:204` | definición del keyFunc dual: rama HS256 (sólo el secreto legacy; T6 la retira) |
| go-shared | `infrastructure/jwtkeyring/jwtkeyring.go:205` | definición del keyFunc dual: guard del método HMAC y del secreto vacío (T6 lo retira con la rama) |
| sales-service | `main.go:78` | verificador de flota #3 (`TenantValidation`), antes `main.go:82`: coordenada corrida por la extracción de la config a `tenantValidationConfig()`; sigue dual vía go-shared v0.18.0 |
| catalog-service | `cmd/api/main.go:322` | verificador de flota #18 (`VerifyTenantCommandJWT`), antes `main.go:325`: coordenada corrida; usa `jwtkeyring` de go-shared v0.18.0 (ya no hay copia local) |

Las filas de §B que apuntaban a las definiciones de `TenantValidation`/`VerifyPlatformJWT` en go-shared
y a los keyFuncs propios de webdata (#15) y catalog (#18) ya no devuelven línea: el selector criptográfico
vive en un único origen, `go-shared/infrastructure/jwtkeyring`, que T6 edita en un solo lugar.

### C. Opciones y decisión

**Opción 1 — Aceptación dual en los verificadores de flota (release de go-shared + bump + wiring propio). ELEGIDA, pendiente de firma.**
El dual aplica a los **cuatro** verificadores del inventario §B con rutas de usuario detrás:
`TenantValidation` y `VerifyPlatformJWT` (go-shared), `AuthMiddleware` de webdata (propio) y
`VerifyTenantCommandJWT` de catalog (propio). Todos adoptan la regla de selección dual de §f del
ADR base, con los tightenings que T4 ya aplicó en iam:

| `alg` del header | Clave de verificación |
|---|---|
| `HS256` (fijado, no la familia HMAC completa) | **sólo** el secreto viejo (`JWT_SECRET`) |
| `RS256` | la pública por `kid` (keyring, ver abajo) |
| `none` / cualquier otro | **rechazo** |

**Jamás** la pública RSA como secreto HMAC (confusión RS/HS); **jamás** `alg:none`. El tightening
de HS384/HS512 (hoy los keyFuncs de la flota aceptan toda la familia) no cuesta nada: el grep
canónico no devuelve **ningún emisor** HS384/HS512 — los tres emisores del inventario (#19-21) son
`jwt.SigningMethodHS256` explícito; queda como evidencia en la bitácora de T5b.

- **Material de la pública — keyring, no archivo único.** La rev. 1 especificaba
  `JWT_PUBLIC_KEY_FILE` (singular): la flota queda clavada a **un solo kid**, y la rotación de la
  clave revocaría a la flota entera (todo 401 hasta que 10 composes se actualicen). Con keyring:
  **`JWT_PUBLIC_KEY_DIR`** — directorio montado con los PEM públicos activos (`acc-*.pem`, uno por
  archivo); el keyFunc deriva el `kid` de cada PEM **por thumbprint RFC 7638** (misma derivación
  que `SigningKeyKID` en iam — la config nunca declara un kid a mano que pueda divergir del
  material) y selecciona por el `kid` del header; `kid` desconocido → 401; dos archivos con el
  mismo kid derivado → **error fatal de arranque** (config rota, no se arranca). El cutover usa un
  directorio con exactamente una clave (kid1): mismo mecanismo, superficie idéntica.
- **Mecánica de rotación de la clave pública en la flota** (condición del gate — hallazgo #4 del
  rechazo de la rev. 1). Con keyring, la rotación tiene su versión de flota del ciclo de §c:
  1. **Publicar**: agregar el PEM de `kid2` al directorio de **cada** consumidor de flota (y al
     JWKS de T7) **antes** de firmar con él — las rutas siguen aceptando `kid1` (keyring con dos
     claves).
  2. **Firmar**: iam flippea el firmador a `kid2` (mismo paso "publicar→firmar" del §c).
  3. **Gracia**: ≥ TTL del access token (15 min) + skew; los tokens `kid1` siguen validando en
     flota (la clave vieja sigue en el keyring).
  4. **Retirar**: quitar `kid1` de cada keyring y del JWKS; desde ese momento los tokens `kid1`
     se rechazan en flota, igual que en iam.
  El costo declarado en §c para Kong (401→refresh en el borde, sin dual de borde) vale igual para
  los consumidores que verifican por kid **con keyring completo**: el 401→refresh del §f es el
  peor caso, y el keyring lo acota a la ventana entre "publicar" y "cada servicio recreado".
- **Validación de arranque fail-closed**: keyring ausente, vacío o con un PEM inválido en
  `GIN_MODE=release` → **no arranca** (patrón §b — explícito, la rev. 1 lo dejaba implícito).
  Paridad con T4: el parser se fija `jwt.WithValidMethods({HS256, RS256})` además del keyFunc,
  como doble control espejo.
- **`JWTSecret` sigue requerido durante la ventana dual** (los tokens S2S HS256 sin `tenant_id`
  siguen entrando por la rama HMAC); el release de T6 lo vuelve opcional y retira la rama.
- **Sin cambio de firma pública de `TenantValidationConfig` que rompa compilación**: se agregan
  campos (`JWTPublicKeyDir`), no se renombra nada.

*Costo*: release minor de go-shared (dos keyFuncs: `TenantValidation` + `VerifyPlatformJWT`) +
bump de **los servicios del inventario con rutas de usuario** + edición de los dos verificadores
propios (webdata `AuthMiddleware`, catalog `VerifyTenantCommandJWT`) + wiring del keyring en cada
compose + wiring del keyring en la flota. Tests por servicio con la negativa de RS256-otra-clave y
`alg:none`.
*Mantenido*: falla-cerrado ante request sin token (el `anonymous` del borde hace de este middleware
el gate real), propagación de claims para `RequireRole`, mismatches 403, `RejectMissingTenant`.
*Este es el único mecanismo que no toca el borde.*

**Opción 2 — Los consumidores dejan de re-verificar en proceso. RECHAZADA.**
Tres razones, las tres verificadas contra el árbol:
1. **Abre el borde.** Con `anonymous: "anonymous-consumer"` en los 14 bloques, Kong no rechaza
   requests sin token; hoy los frena `TenantValidation` (`tenant_validation.go:56-58`). Quitar el
   middleware sin retirar el `anonymous` deja las rutas gateadas **accesibles sin credenciales** —
   un bypass de auth directo. Retirar el `anonymous` son 14 edit points del template más decidir
   qué pasa con cada ruta pública legítima (health, wizard de onboarding, categorías públicas de
   catalog-bff) — blast radius mayor que el de la opción 1, con cambio de contrato de borde.
2. **Rompe la propagación de claims.** El plugin `jwt` de Kong OSS no reenvía claims al upstream:
   `RequireRole` y los controllers que leen `tenant_id`/`namespace`/`roles` se quedan sin fuente
   (mismo problema de drenaje que §e documentó para Kong). Habría que construir un middleware de
   propagación (parsing de claims sin verificación — inaceptable sin borde confiable — o
   request-transformer con la lista de claims en 14 bloques) en los 10 servicios.
3. **Pierde los gates de tenancy.** Mismatch 403 y `RejectMissingTenant` 403 viven en ese
   middleware; retirarlo los borra, contradiciendo el rollout de fail-closed de `PLAT-E46`
   (completado el 2026-09-13 con los 12 servicios en `RejectMissingTenant: true` o condicional).
*Queda como evolución futura*: un verificador de flota por JWKS (fetch con cache del
`/.well-known/jwks.json` de T7) es el punto de llegada natural cuando `account-service` sea
cross-lab y la flota no deba ni tener la pública embebida; no es el mecanismo del cutover.

**Opción 3 — HS256 sigue aceptado en el borde hasta migrar la flota. RECHAZADA — incoherente con T3.**
El firmador ya emite RS256 sólo (T3, firmado; condición C1 del gate de T3: el flip es atómico con
el deploy). Un borde que siga HS256 rechazaría **todos** los tokens nuevos; "aceptar HS256 en el
borde" sólo sostiene a los tokens viejos (≤ 15 min por TTL) y exige reabrir el dual de borde que
§e descartó por irrealizable (`cache_key = { key }` unique; los tokens HS256 vivos no llevan
`kid`). La única versión ejecutable de esta opción es revierte T3 y reintroducir emisión HS256 —
retroceder el cutover ya aprobado.

### D. Emisores HS256 — service tokens firmados con `JWT_SECRET` (afecta a T6)

El criterio (a) corregido también devolvió **tres emisores** que ningún documento de esta épica
había inventariado (filas #19-21 de §B). Ninguno firma tokens de usuario — firman **service
tokens** que otro componente verifica con el mismo secreto compartido:

| Emisor | Verificador | Flujo | Por qué depende del secreto compartido |
|---|---|---|---|
| webdata `service_token.go:94` | catalog `VerifyPlatformJWT` (go-shared) | webdata → `platformGroup` de catalog (PLAT-E39 D8/T9b): service token con `roles: system_admin` | `os.Getenv("JWT_SECRET")` (`:46`); si el secreto compartido se corta, el flujo se rompe |
| onboarding `service_token_provider.go:112` | Kong (borde, `key_claim_name: iss` contra el consumer `iam-service`) + verificación in-process de iam en `/api/v1/tenants*` | onboarding → `lab-kong:8000/iam-service/api/v1/tenants*` con token auto-emitido `iss: iam-service` | `JWT_SECRET` (`setup.go:34`); hace pasar sus tokens por tokens de iam |
| iam `scripts/generate-onboarding-token.go:49` | (idem onboarding) | **DEPRECATED** — fallback CLI del mismo flujo; no corre en runtime | tooling que también depende de `JWT_SECRET` |

**Destino de los emisores — decisión de T6, no de T5a.** El flip de T5 y el corte de T6 retiran la
capacidad de verificar estos tokens HS256: en el borde (Kong RS256-only post-T5) y en los
verificadores in-process (post-T6). T5a no migra nada: la obligación es que T6 **no retire el
secreto de usuario** sin decidir a dónde va cada flujo S2S con destino explícito y deuda escrita —
migrar a API keys por ADR-002 (el S2S de catalog ya corre detrás de `key-auth`+ACL en el borde,
D4), re-firmarlos con un mecanismo propio no compartido, o dejarlos corriendo sobre un secreto
**no-user** distinto, con el riesgo declarado. El `iss: iam-service` falsificado del emisor de
onboarding es un antipatrón que T6 debe resolver, no preservar: el servicio finge ser el emisor de
identidad para pasar un gate que hoy confía en un secreto compartido — con firma asimétrica esa
falsificación **deja de ser posible** (una pública no firma), y el flujo tiene que migrar a algo
honesto (API keys scoped por ADR-002 o token de servicio con `iss` real y verificación que lo
acepte).

### E. Secuencia re-alcanceada por esta decisión

1. **`T5b`** (prerequisito de T5, extendida por esta rev.): release de go-shared con el keyFunc
   dual de §C en **`TenantValidation` Y `VerifyPlatformJWT`** (keyring `JWT_PUBLIC_KEY_DIR`, kid
   por thumbprint, fail-closed en release) + edición del `AuthMiddleware` de webdata (dual con el
   mismo keyFunc/keyring — es un verificador propio, no go-shared) + edición de
   `VerifyTenantCommandJWT` de catalog (dual transitorio: sin él, el flip de T5 rompe en 401 el
   flujo de global product requests el mismo día, porque los tokens reenviados por PIM pasan a ser
   RS256) + bump de los consumidores del inventario con rutas de usuario (los 10 de §B.2 + catalog
   por `platformGroup`) + wiring del keyring en los composes + tests negativos por servicio (RS256
   otra clave → 401; `alg:none` → 401; RS/HS confusión → 401). Sin flip de borde — el lab sigue en
   HS256-emisor-pendiente hasta T5.
2. **T5** (flip de Kong, rebuild+recreate, atómico con el deploy del firmador — C1): su Contrato
   nombra esta decisión (addendum §C opción 1, rev. 2) y su `Depende de` agrega `T5a`+`T5b`.
3. **T6** (cutover, extendida por esta rev.): su alcance incluye a **los cuatro verificadores de
   flota** — el "ningún componente acepta HS256 para tokens de usuario" se verifica también en los
   consumidores bumpados (prueba negativa in-process de HS256 viejo en un consumidor representativo:
   sales por dinero, tenant por montaje global, además de iam), el release de go-shared de T6 deja
   `JWTSecret` opcional y retira la rama HMAC, y el corte incluye el **destino explícito** de los
   emisores de §D (webdata, onboarding y el script deprecado) y del S2S tenant-command de catalog
   (§A.4): migración a API keys por ADR-002 o exclusión del corte con la deuda escrita. La
   auditoría de usos residuales barre con el mismo grep de 5 patrones de §B — el grep es la
   evidencia, no este párrafo.
4. **Exclusión explícita — `pocs/riotless/services/support-service`** (fila #14 de §B; hallazgo #3
   del rechazo de la rev. 1): es un PoC de otro trabajo (riotless), no sirve tráfico de usuario del
   lab, y el flip no lo rompe mientras siga con el secreto viejo durante la ventana. **Queda
   excluido del bump de T5b con esa justificación** — y T6 lo lista en su auditoría de residuales:
   si el PoC sigue vivo al momento del corte, se le retira `JWT_SECRET` (deja de verificar, que
   para un PoC es el estado correcto) o se le aplica el release nuevo; lo que NO se hace es
   ignorarlo. La rev. 1 lo declaraba inexistente: existía en `pocs/` y el grep acotado a `active`
   cumplía su letra sin cumplir su intención — la lección "el grep es la evidencia" se aplicó a
   medias. El criterio (a) corregido incluye `pocs` en el scope del grep.

### Firmas del addendum (gate L4 — `ACC-E03` T5a)

> Estos checkboxes los marca el gate, no el autor del addendum. Sin ambos marcados, T5a NO está
> aprobada y T5/T7 siguen bloqueadas. **Rev. 2**: la rev. 1 fue rechazada por el gate el
> 2026-09-15 (`escalations/2026-09-15_ACC-E03-T5a-signoff.md`); la firma de esta rev. re-avalúa el
> addendum completo (inventario de 5 patrones §B, keyring/rotación §C, emisores §D, secuencia §E).

- [x] **@dev-security GO** — decisión de flota: inventario completo (§B, 25/25 líneas
  clasificadas), opciones evaluadas (§C), especificación del dual con keyring y mecánica de
  rotación de flota (§C opción 1), emisores (§D) y secuencia re-alcanceada (§E).
- [x] **Owner GO** — elección confirmada sobre cómo verifica RS256 la flota antes del flip de T5.

## Firmas (gate L4 — `ACC-E03` T1)

> Estos checkboxes los marca el gate, no el autor del ADR. Sin ambos marcados, T1 NO está
> aprobada y las tareas que dependen de ella no pueden ejecutarse (`[~]` = hecho, esperando firma).

- [x] **@dev-security GO** — algoritmo (RS256, EdDSA descartado con motivo), `kid`/rotación,
  forma del JWKS, cómo verifica Kong (restricción probada contra el binario) y cutover.
  *GO condicionado otorgado 2026-09-08 (escalación `2026-09-08_ACC-E03-T1-signoff.md`); las dos
  condiciones — costo de rotar sin dual de borde en §c y regla de selección dual en §f — fueron
  cerradas por T1b (`ACC-E03`) y verificadas contra este archivo.*
- [x] **Owner GO** — decisión de firma de la raíz de confianza del lab.
  *APROBADO 2026-09-08 (escalación `2026-09-08_ACC-E03-T1-signoff.md`, nota del owner).*