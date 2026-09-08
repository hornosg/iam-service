// Package archtest — Gate de material de firma de ACC-E03 T2 (L4).
//
// Los tests de unitario de la carga (config/signing_key_test.go) prueban la
// FUNCIÓN con claves efímeras. Ninguno pinea los artefactos REALES ni los
// criterios del "Hecho cuando" de T2 — que son de repositorio, no de código:
//
//   (a) `git ls-files | grep -iE 'private|\.pem$|\.key$'` → sin resultados y
//       `git check-ignore <ruta-clave-privada>` devuelve la ruta (está ignorada).
//   (b) `openssl pkey -in <priv> -pubout` produce la pública sin error (par
//       válido, PEM) — este test lo verifica in-process con x509.MarshalPKIX.
//   (c) `.env.example` documenta la var nueva SIN valor real, y
//       `git log -p | grep -i` del patrón "BEGIN .*" + tipo de bloque
//       "PRIVATE KEY" → sin resultados.
//
// Si el par se corrompe, se regenera sin actualizar el registro de kid, alguien
// commitea material, o el boot deja de validar la clave en release, estos tests
// fallan ANTES de T3/T5/T7 — que consumen exactamente este material. Cada caso
// NEGATIVO es la negativa de un criterio del contrato: si el control no está,
// el test falla.
//
// Nota: en un clone fresco (sin keys/) estos tests fallan — es lo correcto:
// sin material la tarea no está hecha. Regenerarlo con
// scripts/generate-signing-keys.sh (idempotente, kid derivado del material).
package archtest

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go/ast"
	"go/parser"
	"go/token"
)

// ---- Fixtures del material real (keys/) ----

// e03t2SigningKeyRegistry es el registro del par generado que exige el contrato
// de T2 ("registrar el kid asignado") — keys/jwt_signing.json.
type e03t2SigningKeyRegistry struct {
	Kid            string `json:"kid"`
	Algorithm      string `json:"algorithm"`
	PrivateKeyFile string `json:"private_key_file"`
	PublicKeyFile  string `json:"public_key_file"`
}

// e03t2Registry carga keys/jwt_signing.json. Si no existe, T2 no está hecha en
// este árbol: no hay par, no hay kid registrado.
func e03t2Registry(t *testing.T) e03t2SigningKeyRegistry {
	t.Helper()
	root := guardWiringModuleRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "keys", "jwt_signing.json"))
	if err != nil {
		t.Fatalf("criterio 'registrar el kid': keys/jwt_signing.json no existe — %v.\n"+
			"Sin el par generado (criterio (b) de T2) no hay material para T3/T5/T7. "+
			"Regenerar con scripts/generate-signing-keys.sh", err)
	}
	var reg e03t2SigningKeyRegistry
	if err := json.Unmarshal(raw, &reg); err != nil {
		t.Fatalf("keys/jwt_signing.json no es JSON válido: %v", err)
	}
	if reg.Kid == "" || reg.PrivateKeyFile == "" || reg.PublicKeyFile == "" {
		t.Fatalf("keys/jwt_signing.json incompleto: kid=%q private_key_file=%q public_key_file=%q",
			reg.Kid, reg.PrivateKeyFile, reg.PublicKeyFile)
	}
	return reg
}

// e03t2LoadPrivateKey parsea la privada real como PEM PKCS#8 RSA (criterio (b)).
func e03t2LoadPrivateKey(t *testing.T, relPath string) *rsa.PrivateKey {
	t.Helper()
	root := guardWiringModuleRoot(t)
	full := filepath.Join(root, relPath)

	info, err := os.Stat(full)
	if err != nil {
		t.Fatalf("la clave privada del par no existe (%s): %v", relPath, err)
	}
	// Gate L4 de T1 (@dev-security): owner-only — sin bits de grupo/otros.
	// El contrato de T2 exige inyección "segura"; una privada 0644 es fuga.
	if perms := info.Mode().Perm(); perms&0o077 != 0 {
		t.Errorf("la clave privada %s tiene permisos %04o — debe ser owner-only (0600/0400): "+
			"cualquier usuario del host puede leer material de firma de la raíz de confianza",
			relPath, perms)
	}

	raw, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("no se puede leer %s: %v", relPath, err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		t.Fatalf("criterio (b): %s no contiene un bloque PEM — el par no es PEM válido", relPath)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("criterio (b): %s no es PKCS#8 (contrato: PEM PKCS#8): %v", relPath, err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		t.Fatalf("criterio (b) + ADR-003 (a): %s es %T — el par debe ser RSA (RS256)", relPath, parsed)
	}
	if key.N.BitLen() < 2048 {
		t.Fatalf("ADR-003 §a: %s es RSA-%d — el contrato exige RSA ≥ 2048", relPath, key.N.BitLen())
	}
	return key
}

// e03t2ThumbprintRFC7638 computa el kid INDEPENDIENTE de la implementación
// (config.SigningKeyKID): SHA-256 del JSON canónico {"e","kty","n"} en orden
// lexicográfico, base64url sin padding (RFC 7638 §3.2). Si la implementación
// divergiera del estándar, ESTE test lo descubre comparando contra el kid
// registrado — no reusa el código bajo prueba.
func e03t2ThumbprintRFC7638(pub *rsa.PublicKey) string {
	members := struct {
		E   string `json:"e"`
		Kty string `json:"kty"`
		N   string `json:"n"`
	}{
		E:   base64.RawURLEncoding.EncodeToString(bigIntBytesLE(pub.E)),
		Kty: "RSA",
		N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
	}
	// json.Marshal respeta el orden de declaración de campos == orden
	// lexicográfico exigido por RFC 7638 (e < kty < n).
	canonical, _ := json.Marshal(members)
	sum := sha256.Sum256(canonical)
	return "acc-" + hex.EncodeToString(sum[:])[:12]
}

// bigIntBytesLE serializa un exponente pequeño big-endian sin ceros a la izquierda.
func bigIntBytesLE(v int) []byte {
	b := make([]byte, 4)
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte(v)
		v >>= 8
	}
	return b[i:]
}

// ---- (b) Par válido: la privada produce la pública registrada ----

// TestACC_E03_T2_SigningKeyPairIsValid verifica el criterio (b) sobre el par
// REAL: la pública derivada de la privada (análogo in-process de
// `openssl pkey -in <priv> -pubout`) es exactamente la pública registrada que
// T3/T5/T7 consumirán.
//
// NEGATIVA implícita: un par desincronizado (privada de otro par, pública
// vieja tras una regeneración) falla acá — T5 cargaría en Kong una pública que
// no verifica NINGÚN token emitido, y el borde del lab entero daría 401.
func TestACC_E03_T2_SigningKeyPairIsValid(t *testing.T) {
	reg := e03t2Registry(t)
	root := guardWiringModuleRoot(t)

	private := e03t2LoadPrivateKey(t, reg.PrivateKeyFile)

	pubRaw, err := os.ReadFile(filepath.Join(root, reg.PublicKeyFile))
	if err != nil {
		t.Fatalf("la clave pública del par no existe (%s): %v — T3/T5/T7 la consumen", reg.PublicKeyFile, err)
	}
	// NEGATIVA de fuga: el archivo registrado como PÚBLICO no debe contener
	// material privado — el contrato dice "NINGÚN byte de clave privada se
	// versiona" y la pública SÍ se versiona; confundirlos versiona la privada.
	if strings.Contains(string(pubRaw), "PRIVATE") {
		t.Errorf("el archivo %s contiene material PRIVADO: la pública es la única parte versionable "+
			"del par — un PEM PRIVATE en una ruta pública es fuga de clave (incidente L4)",
			reg.PublicKeyFile)
	}

	pubBlock, _ := pem.Decode(pubRaw)
	if pubBlock == nil {
		t.Fatalf("criterio (b): %s no es PEM", reg.PublicKeyFile)
	}
	pubParsed, err := x509.ParsePKIXPublicKey(pubBlock.Bytes)
	if err != nil {
		t.Fatalf("criterio (b): %s no es un SubjectPublicKeyInfo válido: %v", reg.PublicKeyFile, err)
	}
	pub, ok := pubParsed.(*rsa.PublicKey)
	if !ok {
		t.Fatalf("criterio (b): %s es %T — debe ser RSA", reg.PublicKeyFile, pubParsed)
	}

	// Oráculo: la pública registrada ES la pública de la privada cargada.
	if pub.N.Cmp(private.PublicKey.N) != 0 || pub.E != private.PublicKey.E {
		t.Errorf("par DESINCRONIZADO: la pública registrada (%s) no corresponde a la privada (%s) — "+
			"todo token firmado por T3 con esta privada fallaría la verificación contra esta pública",
			reg.PublicKeyFile, reg.PrivateKeyFile)
	}
}

// ---- Contrato: "registrar el kid asignado" ----

// TestACC_E03_T2_RegisteredKIDMatchesMaterial verifica que el kid REGISTRADO
// (keys/jwt_signing.json) es el thumbprint RFC 7638 del material REAL.
//
// NEGATIVA: si el par se regenera y el registro queda con el kid viejo, este
// test falla — T3 pondría en el header un kid que no describe el material, y
// los verificadores de T4/T7 no podrían seleccionar la clave por kid.
func TestACC_E03_T2_RegisteredKIDMatchesMaterial(t *testing.T) {
	reg := e03t2Registry(t)

	// ADR-003 §a: la decisión registrada es RS256.
	if reg.Algorithm != "RS256" {
		t.Errorf("ADR-003 (a) decidió RS256 — keys/jwt_signing.json registra algorithm=%q", reg.Algorithm)
	}

	if !regexp.MustCompile(`^acc-[0-9a-f]{12}$`).MatchString(reg.Kid) {
		t.Errorf("kid registrado %q no matchea la forma del ADR-003 §c: acc-<12 hex>", reg.Kid)
	}

	private := e03t2LoadPrivateKey(t, reg.PrivateKeyFile)
	derived := e03t2ThumbprintRFC7638(&private.PublicKey)
	if derived != reg.Kid {
		t.Errorf("kid registrado (%s) ≠ kid derivado del material (%s): el registro quedó desincronizado "+
			"tras regenerar el par — T3 firmaría con un kid que no describe el material",
			reg.Kid, derived)
	}
}

// ---- (a) Cero material privado versionado ----

// e03t2PrivatePathMatcher replica el patrón del criterio (a):
// `grep -iE 'private|\.pem$|\.key$'` sobre los paths de `git ls-files`.
var e03t2PrivatePathMatcher = regexp.MustCompile(`(?i)private|\.pem$|\.key$`)

// TestACC_E03_T2_NoPrivateMaterialVersioned es la NEGATIVA central de T2:
// "NINGÚN byte de clave privada se versiona — una clave privada versionada es
// un incidente L4 y falla la tarea". Ejecuta el criterio (a) tal como está
// escrito: grep sobre ls-files (sin resultados) y check-ignore de la ruta real
// (la devuelve).
//
// Los dos primeros asserts prueban el ORÁCULO del propio test: si el matcher
// no detectara paths ofensores o check-ignore no discriminara, este gate
// pasaría vacío — un test que nunca puede fallar no protege nada (mismo
// argumento de la épica sobre el arch test en ACC-E01).
func TestACC_E03_T2_NoPrivateMaterialVersioned(t *testing.T) {
	root := guardWiringModuleRoot(t)
	reg := e03t2Registry(t)

	// Oráculo 1: el patrón del criterio (a) DETECTA material ofensor.
	for _, offender := range []string{
		"keys/jwt_signing_private.pem", // la ruta real de la clave
		"credentials.key",
		"some/private_thing.go",
	} {
		if !e03t2PrivatePathMatcher.MatchString(offender) {
			t.Fatalf("oráculo roto: el patrón del criterio (a) no detecta %q — "+
				"el gate pasaría aunque la clave estuviera versionada", offender)
		}
	}
	if e03t2PrivatePathMatcher.MatchString("src/main.go") {
		t.Fatal("oráculo roto: el patrón del criterio (a) da falso positivo sobre src/main.go")
	}

	// Criterio (a), parte 1: ningún path versionado matchea material privado.
	lsOut, err := exec.Command("git", "-C", root, "ls-files").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	var versioned []string
	for _, line := range strings.Split(strings.TrimSpace(string(lsOut)), "\n") {
		if line != "" && e03t2PrivatePathMatcher.MatchString(line) {
			versioned = append(versioned, line)
		}
	}
	if len(versioned) > 0 {
		t.Errorf("criterio (a) VIOLADO — material privado versionado (incidente L4): %s",
			strings.Join(versioned, ", "))
	}

	// Criterio (a), parte 2: la ruta real de la privada está ignorada.
	ignOut, ignErr := exec.Command("git", "-C", root, "check-ignore", reg.PrivateKeyFile).Output()
	if ignErr != nil {
		t.Errorf("criterio (a): git check-ignore %s falla con %v — la clave privada NO está "+
			"ignorada por .gitignore; un `git add .` la versiona", reg.PrivateKeyFile, ignErr)
	} else if got := strings.TrimSpace(string(ignOut)); got != reg.PrivateKeyFile {
		t.Errorf("criterio (a): check-ignore devolvió %q, esperaba %q", got, reg.PrivateKeyFile)
	}

	// Oráculo 2: check-ignore NO reporta un archivo versionado (go.mod) —
	// si devolviera 0 para todo, el assert anterior no probaría nada.
	if out, err := exec.Command("git", "-C", root, "check-ignore", "go.mod").Output(); err == nil {
		t.Fatalf("oráculo roto: check-ignore dice que go.mod está ignorado (%s) — "+
			"no discrimina ignorados de versionados", strings.TrimSpace(string(out)))
	}
}

// ---- (c) Historia limpia ----

// TestACC_E03_T2_NoPrivateMaterialInGitHistory ejecuta la segunda parte del
// criterio (c): `git log -p | grep -i` del patrón "BEGIN .*" + tipo de
// bloque "PRIVATE KEY" → sin resultados
// ("nunca estuvo en la historia"). Se usa --all (más estricto que el criterio:
// cubre refs que no son HEAD — la memoria del repo no es sólo master).
//
// NEGATIVA: si en cualquier commit histórico existió un PEM privado, este test
// falla — sacarlo del working tree no lo saca de la historia.
func TestACC_E03_T2_NoPrivateMaterialInGitHistory(t *testing.T) {
	root := guardWiringModuleRoot(t)

	out, err := exec.Command("git", "-C", root, "log", "-p", "--all").Output()
	if err != nil {
		t.Fatalf("git log -p --all: %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) > 0 && line[0] == '+' {
			upper := strings.ToUpper(line)
			if strings.Contains(upper, "BEGIN") &&
				strings.Contains(upper, "PRIVATE KEY") {
				t.Fatalf("criterio (c) VIOLADO: la historia de git contiene material de clave privada "+
					"(línea agregada que matchea \"BEGIN .*\" + \""+
					"PRIVATE KEY\"). Reescribir la historia o rotar el par — "+
					"el material ya está comprometido (incidente L4)")
			}
		}
	}
}

// ---- (c) .env.example documenta la var SIN valor real ----

// TestACC_E03_T2_EnvExampleDocumentsSigningVars verifica la primera parte del
// criterio (c): `.env.example` documenta la var nueva SIN valor real.
//
// NEGATIVA: si JWT_PRIVATE_KEY_FILE aparece con un valor (path real, o peor,
// un PEM inline), el test falla — el archivo está versionado: un valor real
// ahí sería material de firma en git. El patrón es el mismo de las S2S keys
// de ese archivo (documentadas, vacías).
func TestACC_E03_T2_EnvExampleDocumentsSigningVars(t *testing.T) {
	root := guardWiringModuleRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, ".env.example"))
	if err != nil {
		t.Fatalf("criterio (c): no se puede leer .env.example: %v", err)
	}
	content := string(raw)

	if strings.Contains(content, "BEGIN") &&
		strings.Contains(content, "PRIVATE KEY") {
		t.Fatalf("criterio (c) VIOLADO: .env.example contiene un bloque PEM de clave privada — "+
			"material de firma versionado (incidente L4)")
	}

	var documented bool
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue // documentación/comentarios: el contrato los permite
		}
		if name, value, found := strings.Cut(trimmed, "="); found {
			name, value = strings.TrimSpace(name), strings.TrimSpace(value)
			switch name {
			case "JWT_PRIVATE_KEY_FILE":
				documented = true
				if value != "" {
					t.Errorf("criterio (c) VIOLADO: JWT_PRIVATE_KEY_FILE está documentada con valor %q — "+
						"debe ir SIN valor real (path o PEM en un archivo versionado)", value)
				}
			case "JWT_PRIVATE_KEY":
				// Inline es la forma dev-only; en .env.example sólo se admite
				// descomentada y VACÍA (el patrón de las S2S keys).
				if value != "" {
					t.Errorf("criterio (c) VIOLADO: JWT_PRIVATE_KEY inline con valor en .env.example — "+
						"la inline es SOLO desarrollo local y jamás versionada (valor: %.40q...)", value)
				}
			}
		}
	}
	if !documented {
		t.Errorf("criterio (c): .env.example no documenta JWT_PRIVATE_KEY_FILE — el contrato exige "+
			"que la var nueva esté documentada para el que clona el repo")
	}
}

// ---- Contrato: validación de arranque fatal en release (patrón JWT_SECRET) ----

// TestACC_E03_T2_BootValidatesSigningKeyInRelease pinea el wiring del control
// "validación de arranque en el mismo patrón que hoy valida JWT_SECRET al
// bootear (fatal si vacía/insegura en prod)" — NEGATIVA por wiring: la
// negativa de fondo vive en LoadSigningKeyFromEnv (falla cerrada, cubierta por
// sus unit tests), pero sólo sirve si el boot EFECTIVAMENTE la consulta y
// aborta. Si alguien borra la llamada de NewAuthModuleConfigFromEnv, la cambia
// por log.Printf y sigue arrancando, o nadie llama a esa función desde el
// arranque, un deploy release sin clave bootea con HS256 para siempre y
// ningún test unitario lo detecta. Mismo espíritu que TestBootGuardWiring
// (ACC-E02 T8o), por AST sobre auth_module.go + router.go.
func TestACC_E03_T2_BootValidatesSigningKeyInRelease(t *testing.T) {
	root := guardWiringModuleRoot(t)
	authModulePath := filepath.Join(root, "src", "identity", "infrastructure", "config", "auth_module.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, authModulePath, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", authModulePath, err)
	}

	// (1) NewAuthModuleConfigFromEnv consulta la clave de firma.
	// El patrón idiomático del guard es lineal: un AssignStmt
	// `signingKey, err := LoadSigningKeyFromEnv()` seguido del `if err != nil`
	// que decide si abortar (a diferencia de boot_guard, que usa ifStmt.Init).
	// El IfStmt del guard no contiene la llamada: hay que seguir la secuencia.
	var guardFound, guardFatal, guardReleaseAware, guardChecked bool
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "NewAuthModuleConfigFromEnv" {
			return true
		}
		awaitingGuard := false
		for _, stmt := range fn.Body.List {
			switch s := stmt.(type) {
			case *ast.AssignStmt:
				awaitingGuard = false
				for _, rhs := range s.Rhs {
					if isDirectCall(rhs, "LoadSigningKeyFromEnv") {
						guardFound = true
						awaitingGuard = true
					}
				}
			case *ast.IfStmt:
				if !awaitingGuard {
					continue
				}
				awaitingGuard = false
				guardChecked = true
				// El if del guard aborta (log.Fatal*) y es consciente de release
				// (el if anidado `ginMode == "release"` vive en su body).
				ast.Inspect(s.Body, func(o ast.Node) bool {
					if call, ok := o.(*ast.CallExpr); ok {
						if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
							if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "log" &&
								(sel.Sel.Name == "Fatal" || sel.Sel.Name == "Fatalf") {
								guardFatal = true
							}
						}
					}
					if lit, ok := o.(*ast.BasicLit); ok && strings.Contains(lit.Value, "release") {
						guardReleaseAware = true
					}
					return true
				})
			}
		}
		return true
	})

	if guardFound && !guardChecked {
		t.Fatal("test roto: la llamada a LoadSigningKeyFromEnv no está seguida de un `if err` — "+
			"el error de carga se descarta; revisar el patrón del guard en NewAuthModuleConfigFromEnv")
	}

	if !guardFound {
		t.Error("wiring roto: NewAuthModuleConfigFromEnv no llama LoadSigningKeyFromEnv — "+
			"la validación existe pero nadie la consulta al bootear: un deploy release sin clave arranca igual")
	}
	if guardFound && !guardFatal {
		t.Error("wiring roto: el error de LoadSigningKeyFromEnv no termina en log.Fatal* — "+
			"un log.Printf deja arrancar release sin clave de firma (fail-open silencioso)")
	}
	if guardFound && !guardReleaseAware {
		t.Error("wiring roto: el guard de la clave no distingue GIN_MODE=release — "+
			"el patrón del contrato es fatal SOLO en prod (dev sigue booteando en HS256 hasta T3)")
	}

	// (2) Alguien arma la config en el arranque real (no sólo la define).
	routerPath := filepath.Join(root, "src", "router.go")
	routerFile, err := parser.ParseFile(fset, routerPath, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", routerPath, err)
	}
	var wired bool
	ast.Inspect(routerFile, func(n ast.Node) bool {
		if expr, ok := n.(ast.Expr); ok && isCallOnIdent(expr, "config", "NewAuthModuleConfigFromEnv") {
			wired = true
		}
		return true
	})
	if !wired {
		t.Error("wiring roto: src/router.go ya no llama config.NewAuthModuleConfigFromEnv — "+
			"la validación de arranque quedó definida pero descableada del boot real")
	}
}

// isDirectCall dice si expr es una llamada directa (sin selector de paquete)
// a la función fn del mismo paquete — p. ej. LoadSigningKeyFromEnv().
func isDirectCall(expr ast.Expr, fn string) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	ident, ok := call.Fun.(*ast.Ident)
	return ok && ident.Name == fn
}

