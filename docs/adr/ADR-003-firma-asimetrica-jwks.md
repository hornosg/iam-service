# ADR-003: Firma asimétrica RS256 + JWKS para tokens de usuario

**Estado**: Propuesto (pendiente `@dev-security GO` + `Owner GO` — gate L4)
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
- Kong consume PEM estático por credencial (§e): en una rotación, la credencial de
  `iam-service-consumer` se re-templatiza con la pública nueva y se hace rebuild+recreate en el
  mismo paso "publicar→firmar". **No hay dual de borde en rotación**: la ventana la cubre el
  hecho de que el cambio de credencial y el cambio de firmador son el mismo deploy.
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

## Firmas (gate L4 — `ACC-E03` T1)

> Estos checkboxes los marca el gate, no el autor del ADR. Sin ambos marcados, T1 NO está
> aprobada y las tareas que dependen de ella no pueden ejecutarse (`[~]` = hecho, esperando firma).

- [ ] **@dev-security GO** — algoritmo (RS256, EdDSA descartado con motivo), `kid`/rotación,
  forma del JWKS, cómo verifica Kong (restricción probada contra el binario) y cutover.
- [ ] **Owner GO** — decisión de firma de la raíz de confianza del lab.