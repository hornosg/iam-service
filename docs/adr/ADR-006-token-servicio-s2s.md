# ADR-006: Token de servicio S2S — client-credentials RS256 emitido por iam

**Estado**: Propuesto — enmendado tres veces el 2026-10-17/18 (objeciones S-1..S-7 de la primera revisión de `@dev-security`; N-1..N-11 de su segunda pasada y A-1..A-6 de `@dev-architect`; decisión del owner de **dos credenciales**; verificación final de ambos: B-1..B-3, M-1, M-2 de `@dev-architect` y C-1..C-5 de `@dev-security`). Gate L4 pendiente de la verificación final de ambos antes de `ACC-E09` T2.
**Fecha**: 2026-10-17
**Deciders**: @dev-architect, @dev-security (obligatorio, L4), owner (decisiones de `ACC-PROP-001` y del 2026-10-17)
**Épica**: `ACC-E09` (Llamadas S2S estándar — token RS256 de servicio emitido por iam) · **Sucede a**: `ADR-002` (registro `S2S_KEY_*`) · **Reutiliza**: `ADR-003` (keyring, `kid`, JWKS)

## Contexto

Inventario vivo en `management/projects/account-service/epicas/ACC-E09-bitacora.md` (T1, 19 pares).
Lo que obliga a decidir:

- La "`S2S_API_KEY` compartida" es en la práctica el literal por defecto `s2s-internal` (mismo hash en
  los cuatro emisores vivos), escrito en el código de `catalog-bff-service/main.go`. No es un secreto.
- Ese valor no es consumer de Kong: los seis `/internal/*` dan 401 hoy (sondeo vivo 2026-10-17).
  sales→(pim, stock, iam, customer) está roto; onboarding→iam da 404 por URL.
- Las llamadas directas por `lab-network` no pasan por Kong, así que ahí el único control es el del
  receptor. catalog-bff→stock, →pim y →catalog mandan sólo `X-API-Key` + `X-Tenant-ID`, sin
  `Authorization`; stock y pim aplican `TenantValidation` (`RejectMissingTenant`) y responden 401: esas
  llamadas están rotas, no abiertas (sondeo vivo). Ningún receptor valida hoy la identidad del servicio
  emisor.
- El tenant viaja en `X-Tenant-ID` y nadie lo prueba.
- webdata→catalog firmaba HS256 con `JWT_SECRET` (retirado en ACC-E03 T6, deuda T6b).

Decisiones del owner: **modelo B** (token RS256 emitido por iam, verificación local con el keyring) y
**corte servicio por servicio** con modo dual hasta T8 (`ACC-PROP-001`, 2026-10-17); y, tras las
revisiones, **dos credenciales** para las llamadas originadas por un usuario: el token de servicio más el
Bearer del usuario reenviado (ver "Alcance de tenant").

Restricción permanente heredada de ACC-E03 T6: ningún verificador ni go-shared vuelve a tener una
rama HMAC o un secreto compartido de firma.

## Decisión

iam expone `POST /api/v1/s2s/token` (módulo `access`). El emisor se autentica con una **aserción firmada
con su propia clave privada** (`private_key_jwt`, RFC 7523), pide un `audience` y un `scope`, y recibe un
JWT RS256 firmado con la **clave vigente del keyring de ADR-003** (mismo `kid`, mismo JWKS). El receptor
verifica **en proceso** con el keyring (go-shared), sin llamar a iam por request. Kong es defensa en
profundidad, no el verificador autoritativo: hay llamadas directas por `lab-network` que no lo atraviesan.

### Por qué `private_key_jwt` y no un `client_secret` por servicio

Un `client_secret` es un bearer que viaja y se almacena; aunque iam lo guarde hasheado, un secreto
filtrado de un emisor sirve desde cualquier lado y por tiempo indefinido. Con `private_key_jwt` iam sólo
guarda **claves públicas**, la privada de cada emisor nunca sale de su contenedor, la aserción vive
≤ 60 s, está atada a un `aud` fijo y su replay se detecta. La custodia es la de la privada de iam
(ADR-003 §b: PEM PKCS#8 montado, no versionado, RSA ≥ 2048, validación al arranque). Cuesta un par de
claves por **emisor** (cinco: catalog-bff, webdata, onboarding, sales, pim), no por receptor.

Alternativas descartadas: (A) extender el registro `S2S_KEY_*` de ADR-002 — sigue siendo un secreto
por par y obliga a llamar a iam por request para validarlo; (C) mTLS/SPIFFE — fuera de alcance de la
épica (transporte, PLAT-E14).

**Revisión prevista.** La evolución natural son las identidades de workload de k3s (tokens proyectados
de service account, PLAT-E14). Para que ese cambio no toque el token emitido, la autenticación del
cliente queda detrás de un puerto (`IssuerKeyRegistry` + verificador de aserción). Con k3s también se
revisan: el RBAC del ConfigMap o Secret con las públicas de emisores (escribir ahí equivale a poder hacerse
pasar por el emisor, igual que C-T8-4 de ADR-003), el rate limit por IP (detrás de IPs de pod pierde
sentido) y el almacén de `jti` con varias réplicas de iam.

### Identificadores de contrato

Los nombres `iss="iam-service/s2s"`, `token_use="service"`, `aud` de la aserción
`"iam-service/s2s/token"`, el `aud` de iam como receptor y la credencial de Kong son **nombres de
contrato, independientes del nombre del repo**: sobreviven al rename de ACC-E07 (Promoción a plataforma).
Se definen una sola vez como constantes en el núcleo de go-shared (`s2stoken`) y las importan iam y todos
los receptores. Cambiarlos es una migración coordinada, no un efecto lateral de renombrar un servicio.

## Claims

Header: `alg: RS256` (único aceptado), `kid` del keyring vigente, `typ: "s2s+jwt"`.

| Claim | Valor | Regla del receptor |
|---|---|---|
| `iss` | `"iam-service/s2s"` — **distinto** del `"iam-service"` de los tokens de usuario | exige igualdad exacta |
| `sub` | nombre canónico del emisor (`sales-service`, `catalog-bff-service`, …) | se usa para log y autorización; nunca se toma de un header |
| `aud` | **un** nombre canónico de receptor (`stock-service`, `account-service`, `catalog-service`…), string, no array | igual a su propio nombre configurado; array o ausente → 401 |
| `scope` | lista separada por espacios, formato `<recurso>:<acción>` (como ADR-002: `config:read`, `tenant:provision`) | la ruta exige su scope; falta → 403 |
| `token_use` | `"service"` | exige `"service"` |
| `tenant_scope` | `"user"` \| `"system"` | ver "Alcance de tenant" |
| `namespace` | proyecto del emisor según su registro (`mc`); **no** es el prefijo de las claves de config | el receptor lo verifica (T3a), mismo chequeo que hoy hace `VerifyPlatformJWT` |
| `iat`, `nbf`, `exp` | `exp = iat + 300 s`, ambos obligatorios | leeway de reloj 30 s; vencido → 401 |
| `jti` | UUID v4 | auditoría |

**`tenant_scope` lo deriva iam, no lo pide el emisor**: es `system` si y sólo si todos los scopes pedidos son «de sistema» según la política del emisor, y `user` si todos son de usuario; una solicitud que mezcla ambas clases → **403**. El prefijo `catalog.` de las claves de config que `config:read` puede leer es una regla del **receptor** ligada a ese scope (`readNamespaces` en iam), no un claim del token.

El token de servicio **no lleva `tenant_id`, `act` ni `roles`**: nunca declara un tenant ni un usuario. Eso
lo aporta el Bearer del usuario, cuando existe (ver abajo). Por eso es cacheable por `(aud, scope)` y no
depende de ningún usuario.

**Separación de tipos de token (obligatoria, y anterior a toda emisión).** Hoy ningún verificador de
usuario mira `iss` ni `token_use`: `TenantValidation` y `VerifyPlatformJWT` de go-shared y `Parse` /
`authorize` de iam aceptan cualquier RS256 con un `kid` del keyring. Como el token de servicio se firma con
esa misma clave, sin este corte valdría como token de usuario en cualquier ruta que no exija además un
rol. Por lo tanto:
- el verificador S2S rechaza cualquier token sin `token_use=service`, sin `aud` o con
  `iss≠iam-service/s2s`;
- **todos** los verificadores de usuario rechazan `token_use=service` y exigen `iss=iam-service`. El
  universo es el del grep canónico de 5 patrones de ADR-003 §B más los verificadores propios, no sólo
  los de go-shared: `Parse`, `ParseIgnoringExpiry`, `authorize` y `token_revocation` de iam (T2, con el
  rechazo en **un único punto** del adapter, así cubre `Parse` y `ParseIgnoringExpiry`);
  `TenantValidation` y `VerifyPlatformJWT` de go-shared (T3); el `AuthMiddleware` propio de webdata y
  `VerifyTenantCommandJWT` de catalog (T3b, hoy sólo catalog exige `iss=iam-service`); y, hasta T8, Kong
  (`key_claim_name: iss` da 401 a un `iss` sin credencial; desde T8, con la credencial del token de servicio
  creada, el corte pasa a ser la ACL);
- **orden de despliegue obligatorio y verificable**: ningún emisor se monta en vivo (su clave pública
  montada en iam, que es lo que habilita la emisión) hasta que **cada receptor** tenga en su `go.mod` un
  tag de go-shared ≥ el de T3, el contenedor recreado y un test negativo con un token de servicio. Esa
  verificación es el entregable de T3b y la precondición escrita de T4; "depende de" no es un control.

### Alcance de tenant

El token de servicio identifica y autoriza al **servicio**. El tenant y el usuario salen de la otra
credencial:

- **`tenant_scope=user`** (llamadas originadas por un usuario: ventas, tenant-command de pim→catalog): el emisor reenvía
  además el **Bearer del usuario** en `Authorization` y manda el token de servicio en `X-Service-Token`.
  El receptor verifica **los dos** localmente: el Bearer con el verificador de usuario (que rechaza
  `token_use=service`) y el token de servicio con el verificador S2S. El **tenant sale del claim
  `tenant_id` del token de usuario** (firmado por iam), no del header, y los roles y permisos del usuario
  los lee del mismo token. Sin Bearer de usuario válido, un token `user` → **401**. Es el patrón que hoy
  usa pim→catalog (Bearer RS256 del usuario más credencial del servicio).
- **`tenant_scope=system`** (sagas y procesos sin usuario: alta de onboarding, enriquecimiento de webdata,
  refresh de plantillas): el token de servicio va solo, en `X-Service-Token`. Sólo para emisores cuyo
  registro lo permite explícitamente y sólo con los scopes de sistema que el registro enumera para ese
  `aud`. No lleva tenant.

**Lecturas por tenant sin usuario (caso del BFF).** catalog-bff resuelve la política de stock y la config
del catálogo por tenant, también para tráfico anónimo del storefront, y no reenvía ningún Bearer. Para
eso usa `tenant_scope=system` con scopes de **lectura propios** (`config:read` sobre `catalog.*` y lectura
de disponibilidad), y los receptores aceptan `X-Tenant-ID` **sólo en esas rutas**. Riesgo residual declarado:
es el mismo de hoy, porque el storefront ya es público por tenant; el scope no permite escrituras. T4 prueba
los dos caminos (con Bearer y anónimo).

**El scope del servicio no reemplaza la autorización del usuario.** Que sales tenga `stock:reserve` no
habilita la operación si el usuario del Bearer no tiene ese permiso: el receptor aplica el scope del
servicio **y** la autorización del usuario cuando el caso de uso es de usuario.

**Regla de `X-Tenant-ID`** (el receptor, siempre después de verificar firmas, `aud`, `exp` y scope):

| Token de servicio | Bearer de usuario | `X-Tenant-ID` | Resultado |
|---|---|---|---|
| `user` | válido, tenant `T` | ausente | se usa `T` |
| `user` | válido, tenant `T` | `T` | 200 |
| `user` | válido, tenant `T` | `U ≠ T` | **403** + log `s2s_tenant_mismatch` |
| `user` | válido **sin** `tenant_id` | cualquiera | **403**, con independencia de `RejectMissingTenant` del receptor |
| `user` | ausente o inválido | cualquiera | **401** |
| `system` | presente | cualquiera | el Bearer se **ignora** para tenant y roles (nunca amplía lo que da el token `system`) |
| `system` | — | ausente | sólo rutas que no son por tenant |
| `system` | — | `U` | aceptado **sólo** si la ruta lo marca como operación de sistema y el scope está en el registro; si no, 403; siempre logueado con `sub` |

## Emisión y renovación

- Solicitud: `grant_type=client_credentials`,
  `client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer`, `client_assertion`
  (RS256 firmado por el emisor: `iss=sub=<servicio>`, `aud="iam-service/s2s/token"` — identificador
  canónico fijo, nunca una URL —, `iat` y `exp` obligatorios con `exp − iat ≤ 60 s`, `jti`), `audience` y
  `scope`. No hay `subject_token`: el token no se deriva de ningún usuario.
- **Orden de validación en iam (fijo)**: forma de la aserción → firma contra el registro del emisor que
  declara `iss=sub` → `exp`/`iat` → `jti` (insertar si no existe). Lo que no tiene forma de aserción se
  rechaza antes de verificar; ningún `jti` de una aserción sin verificar se escribe.
- iam rechaza con **401** una aserción inválida, vencida o con `jti` ya visto; con **403** un `audience`
  o `scope` fuera del registro del emisor, o `tenant_scope=system` no habilitado.
- **Exposición del endpoint**: se alcanza sólo por `lab-network` directo o por una ruta de Kong
  **dedicada**, sin `jwt`, con rate limit, que no es la `/iam/` pública ni cambia con el corte de T8.
  `/iam/api/v1/s2s/token` por el catch-all público de Kong responde 404 (T2 agrega la ruta de bloqueo y
  una prueba que lo verifica).
- **Rate limit pre-auth**: por IP de origen más un tope global; el cupo **por emisor** se descuenta
  recién después de verificar la firma, para que quien envía basura con `iss=<otro servicio>` no agote el
  cupo de ese servicio. El `iss` sin verificar nunca es clave de un contador.
- **Anti-replay**: los `jti` se guardan en **PostgreSQL** (tabla `s2s_assertion_jti`: PK y `exp`;
  `INSERT … ON CONFLICT DO NOTHING`; purga periódica), con TTL ≥ `exp + 30 s` de leeway. Se elige
  PostgreSQL y no Redis porque iam ya depende de él, no suma una dependencia de disponibilidad y sirve
  con varias réplicas (PLAT-E14). Si el almacén no responde, la emisión **falla cerrada** (503; nunca se
  saltea el chequeo).
- TTL del token: **300 s**. Como el token no depende de ningún usuario, el cliente de go-shared (T3) lo
  cachea por **`(aud, scope)`** y lo renueva a los `exp − 60 s`; ante 401 del receptor descarta la
  entrada y re-emite una sola vez. Una emisión por destino cada ~4 minutos, no una por usuario.
- Auditoría: cada emisión loguea `sub`, `aud`, `scope`, `tenant_scope`, `jti`, `kid`. **Nunca** el token
  ni la aserción.

## Registro

Reemplaza a `ServicePolicy` + `S2S_KEY_<SERVICE>` de ADR-002.

- **Política, versionada en el repo de iam** (no es secreto), organizada por namespace de proyecto para
  que iam, como servicio de plataforma, admita otros proyectos sin tocar las entradas de MC: por emisor,
  `namespace`, lista de `aud` permitidos y, por cada `aud`, los scopes permitidos y si admite
  `tenant_scope=system`. El cambio de política es un cambio de código revisado, como hoy `ServicePolicy`.
- **Contrato entre las dos fuentes de scopes**: la política de iam define emisor→aud→scopes y cada
  receptor define ruta→scope. Para que no diverjan, cada tarea de migración entrega un test de contrato
  que verifica que todo scope de la política existe en alguna ruta del receptor y viceversa.
- **Clave pública del emisor**, montada en iam desde `infra/s2s-keys/<servicio>/` (sólo `PUBLIC KEY`,
  RSA ≥ 2048, `kid` derivado del material). Hasta dos claves por emisor para rotar. Un emisor sin clave
  pública cargada no puede autenticarse (fail-closed).
- **Keyring de emisores separado del de verificación.** Estas públicas viven en un registro propio del
  módulo `access`, indexado por emisor. **Nunca** se cargan en el keyring de `Parse` de iam, ni en
  `JWT_PUBLIC_KEY_DIR` de ningún receptor, ni se publican en el JWKS: si una pública de emisor llegara a
  cualquiera de ellos, ese emisor podría firmar tokens de usuario o de servicio con su propia privada. La
  aserción se verifica buscando el `kid` **dentro del registro del emisor que declara `iss=sub`**, nunca
  en un mapa global. T2 lo prueba con un test negativo (un token firmado con la clave de un emisor da 401
  en `Parse` y en el verificador S2S) y con una validación al arranque que rechaza una privada de emisor
  puesta en un `JWT_PUBLIC_KEY_DIR`. T2 entrega además `s2s-keygen <servicio>`: genera el par, deriva el
  `kid` y deja la pública en `infra/s2s-keys/<servicio>/`.
- Registrar un emisor nuevo = generar su par, montar la privada en su contenedor, la pública en iam y
  agregar su entrada de política. Sin rama de secreto compartido.
- **T2 registra sólo a catalog-bff-service** (el piloto de T4). Cada emisor entra en la tarea que lo
  migra, con scopes concretos y verificados. Ninguna entrada puede contener `system:admin`,
  `tenant:admin` ni un comodín equivalente. Las operaciones que admiten `tenant_scope=system` se
  enumeran **por receptor**: la lista de rutas vive en el repo del receptor, que es quien la aplica, y
  cada operación de sistema tiene un scope propio que sólo existe con `tenant_scope=system`. La tabla es
  indicativa (cada tarea la fija al implementar):

| Emisor | `aud` | Scopes iniciales | `system` |
|---|---|---|---|
| catalog-bff-service | stock-service, account-service, pim-service, catalog-service | lecturas propias: `stock:read`, `config:read` (el receptor restringe a `catalog.*`), lecturas de catálogo | sí, sólo esos scopes de lectura |
| webdata-service | catalog-service | escrituras de catálogo de plataforma, enumeradas una por una en T5 (no `system_admin`) | sí |
| onboarding-service | account-service, tenant-service, pim-service, notifications | `tenant:provision`, `tenant:bootstrap`, `notification:send` (T6 enumera el resto) | sí |
| sales-service | pim-service, stock-service, account-service, customer-service | `product:read`, `stock:reserve`, `tenant:read`, `customer:read` | no |
| pim-service | catalog-service, account-service | `catalog:tenant-command`; `ratelimit:read` si es una llamada de fondo sin usuario necesita `system` (T7b lo decide) | por decidir en T7b |

### Ubicación en la hexagonal de iam

La emisión vive en el módulo `access`. `access` declara sus puertos en `access/domain/port`:
`ServiceTokenSigner`, `AssertionReplayStore`, `IssuerKeyRegistry` y `ServiceTokenIssuer`. Los adaptadores
se conectan en el composition root (`router.go`). **Ningún archivo de `src/access` importa
`src/identity/...`** ni al revés: `src/arch_test.go` suma las filas `{identity, access}` y
`{access, identity}` a `prohibiciones`. Los claims de servicio no entran en el value object `TokenClaims`
de identity (que son claims de usuario); el firmador expone una firma genérica sobre `jwt.Claims`, o el
material de firma pasa a un kernel interno (`src/shared/signing`) que ninguno de los dos módulos posee.
Los literales de contrato (`iss`, `token_use`) viven en go-shared, nunca en `access`.

## Revocación

1. **Retirar el registro** del emisor (su entrada de política o su clave pública) y reiniciar iam: desde
   ese momento iam responde 401 a toda solicitud de token de ese emisor.
2. **Expiración corta**: los tokens ya emitidos siguen válidos hasta su `exp` — **como máximo 300 s +
   30 s de leeway**. Pasado eso, cualquier receptor responde 401.
3. **Corte inmediato (incidente)**: cada receptor admite `S2S_DENY_SUB`, una lista de nombres canónicos
   de emisor separados por comas, que rechaza con 401 sin esperar el vencimiento. Es configuración, no un
   secreto. **Falla cerrada sin tumbar a los usuarios**: un valor que no se puede parsear hace que el
   receptor **rechace todo token de servicio**, mantenga el tráfico de usuarios y emita log y métrica en
   alto; no hace que ignore la lista ni que no arranque. Pasa a los runbooks de incidente.
4. Comprometida la **privada de un emisor**: revocación según 1 + 3 y alta de un par nuevo.
   Comprometida la **privada de iam**: rotación de emergencia de ADR-003 (afecta usuarios y servicios
   por igual, porque es la misma clave).
5. El token de servicio no extiende la sesión de ningún usuario, porque no lleva usuario. **El Bearer del
   usuario se revoca sólo en iam** (`token_revocation`): go-shared no consulta revocación, así que stock,
   pim, customer y catalog aceptan un Bearer revocado o con sesión cerrada hasta su `exp` de 15 min. Hoy
   ese Bearer ya viaja a esos receptores; con este modelo además es la autoridad de tenant y roles en S2S.
   Riesgo residual declarado, mitigación futura: Bearer con `aud` o intercambio RFC 8693 (PLAT-E14).

No hay lista de revocación consultada por request: a propósito, para no reintroducir una llamada a iam
por request. El riesgo residual es la ventana de ≤ 330 s para el token de servicio, que se cubre con el punto 3; para el Bearer de usuario es de hasta 15 min fuera de iam (§5).

## Rotación

- **Clave de firma de iam**: la misma de ADR-003 §c — publicar el `kid` nuevo, **esperar 300 s** antes
  de firmar con él, gracia para el `kid` viejo, retirarlo. El TTL de 300 s del token de servicio cabe en
  la gracia existente; no hace falta un procedimiento aparte. El verificador S2S usa el mismo keyring
  inmutable que `Parse` (ACC-E03 T8), así que retirar un `kid` revoca también los tokens de servicio
  firmados con él (la prueba viva de C-T8-3/C4, pendiente en ACC-E03, cubre ambos tipos).
- **Clave de un emisor**: montar la pública nueva en iam junto a la vieja (dos por emisor), cambiar la
  privada del emisor, retirar la pública vieja. Como sólo firma aserciones de ≤ 60 s, la gracia es de
  un minuto.

## Modelo de amenazas

| Amenaza | Qué la frena | Qué se rechaza y con qué código |
|---|---|---|
| **Servicio comprometido** (atacante con la privada del emisor o dentro de su contenedor) | El token sólo vale para los `aud` y scopes de **su** registro; el tenant y el usuario salen del Bearer del usuario, que el servicio no puede fabricar; no puede firmar como iam ni como otro emisor | iam: 403 a `aud`/scope fuera del registro; receptor: 403 a scope insuficiente, 401 a un Bearer de usuario ausente o inválido y 403 a tenant discordante. Contención por revocación (≤ 330 s, o inmediata con `S2S_DENY_SUB`). Riesgo residual declarado: dentro de su registro y con los Bearer de usuario que ya tiene, actúa como el servicio legítimo; con `system` habilitado actúa sin usuario sólo en las operaciones de sistema enumeradas |
| **Bearer reenviado fuera de registro** (servicio comprometido que manda sólo el Bearer de un usuario a otra ruta de usuario de cualquier servicio) | Ninguna del modelo: el Bearer no tiene `aud`. Cada receptor nuevo de Bearers amplía la superficie (confused deputy aguas abajo) | Aceptado como riesgo residual declarado; ya existe hoy. Mitigación futura: Bearer con `aud` o intercambio RFC 8693 (PLAT-E14) |
| **Token de servicio robado + Bearer propio del atacante** | Da al usuario los scopes del servicio sobre su propio tenant (p.ej. `stock:reserve` sin pasar por las reglas de sales). Se contiene con `X-Service-Token` fuera de las rutas públicas, `exp` 300 s, `S2S_DENY_SUB` y la regla de que el scope no reemplaza la autorización del usuario | Rutas públicas de Kong quitan `X-Service-Token` (T4); 403 a lo que el usuario no puede hacer |
| **Token robado** (el de servicio: log, proxy, volcado de memoria) | `exp` 300 s, `aud` único, nunca se loguea; por sí solo no da tenant ni usuario | **Tras revocar** el emisor y vencer el token: **401** en todo receptor. Antes del vencimiento: 401 sólo si el emisor está en `S2S_DENY_SUB`; si no, es aceptado — riesgo residual declarado, acotado a 330 s y a un receptor, y sin identidad de usuario |
| **Bearer de usuario robado** | `exp` 15 min; revocación efectiva sólo en iam y, en los demás receptores, hasta el `exp` (ver Revocación §5) | Igual que hoy |
| **Confusión de `aud`** (usar contra pim un token emitido para stock) | `aud` string único, comparado con el nombre configurado del receptor | **401** con `aud` ajeno, en array o ausente |
| **Confusión de tipo** (token de usuario como token de servicio, o al revés) | `iss` distinto + `token_use` en todos los verificadores del universo de arriba | Verificador S2S: **401** a un token de usuario. Verificadores de usuario: **401** a `token_use=service`. La precondición verificable es la de T3b/T4 |
| **Tenant discordante** (`X-Tenant-ID` distinto del claim del Bearer) | Regla de `X-Tenant-ID` | **403** + log `s2s_tenant_mismatch` |
| **Replay del token de servicio** | `exp` corto + `aud` | Dentro de la ventana el token es reutilizable por diseño (el cliente lo cachea); fuera de ella **401**. Riesgo igual al de token robado |
| **Replay de la aserción** ante iam | `exp − iat ≤ 60 s` + `jti` en PostgreSQL con TTL ≥ `exp + 30 s`; falla cerrado | **401** a un `jti` repetido; **503** con el almacén caído |
| **DoS entre emisores** (basura con `iss` ajeno) | Rate limit por IP y global antes de verificar; cupo por emisor sólo tras verificar | 429 al excedente anónimo; el cupo de un emisor legítimo no se toca |
| **Downgrade de algoritmo** (`alg: none`, HS256 con la pública como secreto) | `WithValidMethods(["RS256"])` en emisor y receptor, sin rama HMAC | **401** |
| **Header de tenant inyectado por el borde** | El tenant sale del claim; el header sólo se compara | 403 si discrepa |

## Transición (modo dual) y Kong

- Durante T4–T7 cada receptor acepta **el token o la credencial heredada**, con el modo configurable
  por receptor y logueado por request (`s2s_auth=token|legacy_key`). La credencial heredada es la que ya
  valida cada receptor hoy; el modo dual no agrega un camino nuevo. T8 lo retira.
- **Precedencia (sin camino de downgrade)**: la credencial se elige **por header, no por `iss`**. El token
  de servicio se acepta sólo desde `X-Service-Token`; el de usuario sólo desde `Authorization`
  (`token_use=service` en `Authorization` → 401; un token de usuario en `X-Service-Token` → 401). El
  combinador del receptor decide por la **presencia de `X-Service-Token`**: si llega, se verifica el token de
  servicio y, según `tenant_scope`, el Bearer con el verificador de usuario; si llega y es inválido, la
  respuesta es **401 sin caer a la credencial heredada**. Sin `X-Service-Token`, rige la credencial
  heredada, que es la que cada receptor valida **hoy** (ningún receptor valida `S2S_API_KEY`; el literal
  `s2s-internal` nunca es una credencial válida). Stock y pim tienen hoy como heredada un JWT de usuario.
- **Reversibilidad por servicio**: el emisor tiene un modo por destino (`S2S_MODE_<DESTINO>=token|legacy`),
  de modo que revertir un par es un cambio de configuración, no un redeploy del emisor.
- **Kong, por par y no al final.** El cambio de la ruta de Kong de cada par (de `key-auth` a `jwt`, o el
  par pasa a ir directo por `lab-network`) se hace **en la tarea que lo migra**; si no, sales y onboarding
  darían 401 en cuanto dejan la key. T8 sólo retira lo que quede. Las rutas internas de servicio usan `jwt`
  con `header_names: [X-Service-Token]` **únicamente** (si queda el default, Kong tomaría el Bearer del
  usuario y lo mandaría al consumer de usuario) y `uri_param_names: []` (las de usuario aceptan `?jwt=`,
  que llevaría el token a los logs de URL), con una credencial `key: "iam-service/s2s"` y la misma
  `rsa_public_key`. Las credenciales jwt son globales al consumer, así que al existir esa credencial el
  único corte entre rutas de usuario y de servicio pasa a ser la ACL: el consumer nuevo está **sólo en
  `service-internal`, nunca en `authenticated`**, y se prueba que toda ruta con `jwt` tiene su `acl`. Los
  consumers S2S actuales (sales, onboarding, catalog-bff) sí tienen `authenticated`
  (`kong.yml.template:915-935`): ese patrón **no se copia**. Un único consumer compartido pierde el rate
  limit y el log por emisor en el borde; el receptor los tiene por `sub`. El endpoint de token tiene su ruta
  dedicada sin `jwt`.
- **Las rutas públicas de Kong quitan `X-Service-Token`** (`request-transformer` con `remove.headers`): un
  token de servicio sólo entra por `lab-network` o por las rutas internas. Es precondición de T4, con una
  prueba: un `curl` público con un `X-Service-Token` válido no llega al upstream.
- La `S2S_API_KEY` con default `s2s-internal` se elimina de los composes y del código en T8 (con un grep
  del literal como criterio); hasta entonces no se le da ningún permiso nuevo.

## Contrato de go-shared (T3a y T3)

- **Núcleo `infrastructure/s2stoken` (T3a, con su propio tag, anterior a T2** porque iam importa de él las
  constantes de contrato y el verificador S2S**)**, sin gin ni `net/http`, como `jwtkeyring`: tipos de claims,
  constantes de contrato, `Verify(token, cfg) (Principal, error)` y errores tipados que separan no
  autenticado (401) de prohibido (403). Usa un `Keyfunc` sin el parámetro `secret` (el de `jwtkeyring`
  recibe un secreto ignorado y Deprecated; T3a agrega la variante sin él o adelanta su retiro).
- **Logs**: `X-Service-Token` se redacta en toda salida de headers (middlewares, Kong, Promtail), igual que
  `Authorization`; el «nunca se loguea» cubre también el lado receptor.
- **`s2stoken/client`** (T3): carga de la privada del emisor, firma de la aserción, caché por `(aud, scope)`,
  transporte HTTP, timeout, circuit breaker y la métrica `s2s_token_issue_errors_total`.
- **Adaptadores**: un middleware gin (`middleware/s2sauth`, con el combinador) y uno `net/http` para
  webdata, que usa chi. Un receptor puro no enlaza la carga de claves privadas.

## Consecuencias

- Desaparece el secreto compartido S2S y la identidad del emisor pasa a ser criptográfica.
- Cada emisor suma un par de claves a custodiar (cinco); iam suma un endpoint, un registro y una tabla.
- **Disponibilidad**: iam sale del camino caliente. Una caída de iam impide **emitir** tokens de servicio
  nuevos, no verificar: los vigentes siguen sirviendo hasta 300 s y, como se cachean por `(aud, scope)`,
  hay una emisión por destino cada ~4 minutos y no una por usuario. El flujo de venta depende de iam sólo
  en esa renovación. El cliente degrada con error explícito (timeout y circuit breaker), no con la key
  vieja.
- Cada llamada de usuario lleva dos credenciales; el receptor verifica dos firmas locales.
- `ADR-002` queda sucedido por éste al cerrar T8 (T8 agrega el puntero).
- Decisión pendiente antes de T6: si la saga de onboarding se mueve a iam (ACC-E05, Onboarding genérico),
  el par onboarding→iam pasa a ser una llamada en proceso y no se cablea token para esa parte.

## Firmas (gate L4 — `ACC-E09` T1)

- `@dev-security`, primera revisión (2026-10-17): modelo aprobado, objeciones S-1..S-7 incorporadas.
- `@dev-security`, segunda pasada (2026-10-17): aprobado con condiciones N-1..N-11; N-1 y N-2 se resuelven
  de raíz con el modelo de dos credenciales (el token de servicio no lleva usuario ni tenant); N-3 → T3b y
  precondición de T4; N-4/N-5 en "Emisión y renovación"; N-6 desaparece (el token no depende del
  `subject_token`); N-7/N-8 en Revocación y Emisión; N-9 en "Separación de tipos"; N-10 en Kong y T8;
  N-11 corregido en el inventario.
- `@dev-architect` (2026-10-17): aprobado con condiciones A-1..A-6 (verificación final 2026-10-18: **APROBADO**, tres restos de texto corregidos); A-1 por el modelo de dos credenciales,
  A-2 en "Ubicación en la hexagonal", A-3 en "Separación de tipos" y T3b, A-4 en "Transición",
  A-5 en "Emisión y renovación" (PostgreSQL), A-6 en "Identificadores de contrato".
- Verificación final (2026-10-18), `@dev-architect`: aprobado con condiciones B-1 (lecturas por tenant sin
  usuario → `system` con scopes de lectura de catalog-bff), B-2 (T3a antes de T2), B-3 (Kong por par), M-1
  (combinador por header) y M-2 (claim `namespace` vs prefijo de config) — incorporadas arriba.
  `@dev-security`: aprobado con condiciones C-1 (credencial por header), C-2 (`tenant_scope` derivado por
  iam y fila `system` + Bearer), C-3 (Bearer sin `tenant_id` → 403), C-4 (revocación del Bearer y
  reenvío, declarados como riesgo residual), C-5 (la épica alineada con el ADR sobre la credencial
  heredada) — incorporadas arriba; C-6..C-11 son criterios de T3, T3b, T4 y T8 (ver la épica).
- `@dev-security`, confirmación acotada (2026-10-18): **APROBADO** con la corrección de la sección Riesgos de la épica y tres retoques menores, aplicados.
- Cierre de T1: ambos revisores conformes (2026-10-18).
