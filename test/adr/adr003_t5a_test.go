//go:build adrgate

// Package adrtest vuelve ejecutables los tres criterios "Hecho cuando" de
// ACC-E03 T5a (ceremonia L4) — el mismo movimiento de adr003_t1b_test.go: la
// decisión de flota vive en un documento, y un documento sin verificador
// ejecutable puede pudrirse en silencio (hallazgo fundante de la propia T5a:
// el grep no vio a TenantValidation porque vivía fuera del repo).
//
// Contrato de ACC-E03 T5a (ACC-E03-firma-asimetrica-jwks.md), tal como está
// escrito — este test verifica el CONTRATO, no el estado actual del addendum:
//
//	(a) el addendum §B clasifica CADA línea que devuelve el grep canónico de
//	    5 patrones (corrección del rechazo del gate, 2026-09-15: la rev. 1 del
//	    addendum sólo buscaba 'TenantValidation(' y perdió a los verificadores
//	    que parsean el JWT por su cuenta — VerifyPlatformJWT, AuthMiddleware de
//	    webdata, VerifyTenantCommandJWT — y a los emisores de service tokens):
//	    grep -rnE --include='*.go' --exclude-dir=node_modules --exclude-dir=vendor
//	      'TenantValidation\(|VerifyPlatformJWT\(|VerifyTenantCommandJWT\(|SigningMethodHMAC|SigningMethodHS256'
//	      active platform pocs | grep -v _test.go
//	      | grep -vE 'func (TenantValidation|VerifyPlatformJWT|VerifyTenantCommandJWT)\('
//	    corrido desde $DEVY_PATH; una línea sin clasificar = inventario
//	    incompleto = tarea FALLA;
//	(b) el addendum tiene los checkboxes `@dev-security GO` y `Owner GO`
//	    MARCADOS en su propia sección de firmas; sin ambos es un L4 sin firma
//	    = tarea FALLA. CUIDADO: el ADR ya contiene `- [x] **@dev-security GO**`
//	    por la firma de T1 — un chequeo GLOBAL de firmas daría falso positivo;
//	    el verificador se acota a "### Firmas del addendum";
//	(c) el Contrato de T5 en la épica nombra la opción ELEGIDA del addendum §C;
//	    si T5 describe el flip sin la decisión de flota, se ejecutaría sobre el
//	    plan viejo = tarea FALLA.
//
// Los TestContratoT5a_* corren contra los archivos reales. Los
// TestNegativoT5a_* mutan el texto real (borran una fila de §B, desmarcan las
// firmas, tapan la decisión en T5) y exigen que el verificador RECHACE — un
// verificador de documento que nunca falló no prueba nada.
//
// El grep canónico se EJECUTA de verdad (no se mockea): el criterio (a) del
// contrato es contra la salida real de ese comando, y un call site nuevo que
// nadie agregó al addendum debe romper el test en el próximo `go test`, no el
// día que alguien se acuerde de re-greppear.
package adrtest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// --------------------------------------------------------------------- rutas

// devyPath resuelve la raíz del lab: $DEVY_PATH (contrato ADR-001) con fallback
// a dos niveles arriba del module root (moduleRoot = $DEVY_PATH/platform/iam-service).
func devyPath(t *testing.T) string {
	t.Helper()
	if devy := os.Getenv("DEVY_PATH"); devy != "" {
		return devy
	}
	return filepath.Join(moduleRoot(t), "..", "..")
}

// ------------------------------------------------------------------ inventario

// seccionAddendum extrae el bloque del addendum T5a (desde su header hasta el
// próximo "## " de nivel superior). El ADR contiene otras firmas (T1) y otros
// addendums: todo chequeo de T5a debe quedar acotado a ESTE bloque.
func seccionAddendum(adr string) (string, error) {
	i := strings.Index(adr, "## Addendum — Verificadores de flota")
	if i < 0 {
		return "", fmt.Errorf("ADR-003: no se encontró el addendum de T5a")
	}
	resto := adr[i:]
	if j := strings.Index(resto, "\n## "); j > 0 {
		resto = resto[:j]
	}
	return resto, nil
}

// seccionB devuelve el bloque del inventario de call sites (§B) del addendum.
// El contrato exige que el listado esté EN §B — un call site mencionado de
// pasada en §A o §C no es el inventario.
func seccionB(addendum string) (string, error) {
	i := strings.Index(addendum, "### B.")
	if i < 0 {
		return "", fmt.Errorf("addendum T5a: no se encontró la sección §B (inventario)")
	}
	resto := addendum[i:]
	if j := strings.Index(resto, "\n### "); j > 0 {
		resto = resto[:j]
	}
	return resto, nil
}

// callSite es un resultado del grep canónico, partido como lo exige §B:
// servicio (directorio del repo) y archivo:línea (cola relativa al root del
// servicio). §B los lista como `src/main.go:52` junto al nombre del servicio —
// es la identidad del call site que el contrato pide listar.
type callSite struct {
	servicio string
	archivo  string
}

// parseCallSites traduce la salida cruda del grep (path:line:contenido) a
// callSite. Los call sites viven en active/mercado-cercano/services/<svc>/,
// platform/<svc>/ o pocs/**/services/<svc>/; las definiciones de los keyFuncs
// viven en active/mercado-cercano/libs/go-shared/ y se clasifican como
// servicio "go-shared" (§B las inventaria igual que los call sites).
func parseCallSites(salidaGrep string) []callSite {
	var sitios []callSite
	for _, l := range strings.Split(strings.TrimSpace(salidaGrep), "\n") {
		if l == "" {
			continue
		}
		partes := strings.SplitN(l, ":", 3)
		if len(partes) < 2 {
			continue
		}
		ruta, linea := partes[0], partes[1]
		comp := strings.Split(ruta, "/")
		for k := 0; k < len(comp)-1; k++ {
			if comp[k] == "libs" && k+1 < len(comp) {
				sitios = append(sitios, callSite{
					servicio: comp[k+1],
					archivo:  filepath.Join(comp[k+2:]...) + ":" + linea,
				})
				break
			}
			if (comp[k] == "services" || comp[k] == "platform") && k+1 < len(comp) {
				sitios = append(sitios, callSite{
					servicio: comp[k+1],
					archivo:  filepath.Join(comp[k+2:]...) + ":" + linea,
				})
				break
			}
		}
	}
	return sitios
}

// verificarInventario implementa el criterio (a): cada call site del grep
// canónico debe aparecer en §B con su servicio y su archivo:línea. Devuelve
// los call sites faltantes — vacío = inventario completo.
func verificarInventario(seccionInventario string, sitios []callSite) []string {
	var faltantes []string
	for _, s := range sitios {
		encontrado := false
		for _, l := range strings.Split(seccionInventario, "\n") {
			if strings.Contains(l, s.servicio) && strings.Contains(l, "`"+s.archivo+"`") {
				encontrado = true
				break
			}
		}
		if !encontrado {
			faltantes = append(faltantes, s.servicio+"/"+s.archivo)
		}
	}
	return faltantes
}

// grepCanonico ejecuta el grep canónico del contrato desde $DEVY_PATH — el
// mismo comando que §B del addendum documenta verbatim. Falla ruidoso: sin
// grep no hay criterio (a) verificable.
func grepCanonico(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("sh", "-c",
		`grep -rnE --include='*.go' --exclude-dir=node_modules --exclude-dir=vendor 'TenantValidation\(|VerifyPlatformJWT\(|VerifyTenantCommandJWT\(|SigningMethodHMAC|SigningMethodHS256' active platform pocs | grep -v _test.go | grep -vE 'func (TenantValidation|VerifyPlatformJWT|VerifyTenantCommandJWT)\('`)
	cmd.Dir = devyPath(t)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 && len(out) == 0 {
			t.Fatal("el grep canónico devolvió 0 líneas: o los verificadores/emisores dejaron de existir (re-alcancear T5a), o la command falló")
		}
		t.Fatalf("grep canónico (criterio a) falló al ejecutarse: %v", err)
	}
	return string(out)
}

// ---------------------------------------------------------- firmas del gate

// seccionFirmasAddendum acota a "### Firmas del addendum (gate L4 — T5a)".
// NO sirve un chequeo global: el ADR ya tiene `- [x] **@dev-security GO**` de
// la firma de T1 y daría falso positivo sobre un addendum sin firmar.
func seccionFirmasAddendum(addendum string) (string, error) {
	i := strings.Index(addendum, "### Firmas del addendum")
	if i < 0 {
		return "", fmt.Errorf("addendum T5a: no se encontró la sección de firmas del gate")
	}
	resto := addendum[i:]
	if j := strings.Index(resto, "\n## "); j > 0 {
		resto = resto[:j]
	}
	return resto, nil
}

// verificarFirmas implementa el criterio (b): los dos checkboxes del gate
// deben estar marcados `- [x]` DENTRO de la sección de firmas del addendum.
// Devuelve los GO que faltan — vacío = firmado.
func verificarFirmas(seccionFirmas string) []string {
	var faltan []string
	for _, go_ := range []string{"@dev-security GO", "Owner GO"} {
		marcado := false
		for _, l := range strings.Split(seccionFirmas, "\n") {
			if strings.Contains(l, go_) && strings.Contains(l, "- [x]") {
				marcado = true
				break
			}
		}
		if !marcado {
			faltan = append(faltan, go_)
		}
	}
	return faltan
}

// ------------------------------------------------- decisión en el Contrato T5

// bloqueT5 devuelve el texto de la tarea T5 de la épica hasta la próxima tarea.
func bloqueT5(epica string) (string, error) {
	i := strings.Index(epica, "T5 · Kong verifica")
	if i < 0 {
		return "", fmt.Errorf("épica ACC-E03: no se encontró el bloque de la tarea T5")
	}
	resto := epica[i:]
	if j := strings.Index(resto, "Depende de:"); j > 0 {
		resto = resto[:j]
	}
	return resto, nil
}

// seccionC devuelve el bloque de opciones y decisión (§C) del addendum.
func seccionC(addendum string) (string, error) {
	i := strings.Index(addendum, "### C. Opciones y decisión")
	if i < 0 {
		return "", fmt.Errorf("addendum T5a: no se encontró la sección §C (opciones y decisión)")
	}
	resto := addendum[i:]
	if j := strings.Index(resto, "\n### "); j > 0 {
		resto = resto[:j]
	}
	return resto, nil
}

// verificarDecision implementa el criterio (c) cruzando los DOS documentos:
// el Contrato de T5 debe nombrar la opción que §C del addendum marca ELEGIDA
// — no "una" opción. Un T5 que cite "opción 2" mientras §C elige la 1
// ejecutaría el flip sobre el plan viejo, que es exactamente lo que el
// contrato quiere impedir.
func verificarDecision(bloque string, secC string) error {
	contrato := ""
	for _, l := range strings.Split(bloque, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "Contrato:") {
			contrato = strings.TrimSpace(l)
			break
		}
	}
	if contrato == "" {
		return fmt.Errorf("el bloque T5 de la épica no tiene línea 'Contrato:'")
	}

	re := regexp.MustCompile(`(?im)^\*\*Opción (\d+) .*(ELEGIDA)`)
	elegidas := re.FindAllStringSubmatch(secC, -1)
	if len(elegidas) != 1 {
		return fmt.Errorf("§C del addendum debe declarar EXACTAMENTE una opción ELEGIDA; encontradas %d", len(elegidas))
	}
	opcion := elegidas[0][1]

	// El contrato de T5 nombra esa opción y referencia el addendum (un
	// "opción 1" suelto en otro contexto no cierra el criterio).
	nombre := fmt.Sprintf("opción %s del addendum", opcion)
	if !strings.Contains(strings.ToLower(contrato), nombre) {
		return fmt.Errorf("el Contrato de T5 no nombra la opción %s ELEGIDA del addendum §C (buscado %q) — se ejecutaría sobre el plan viejo", opcion, nombre)
	}
	return nil
}

// ------------------------------------------------- contrato: archivos reales

// TestContratoT5aCriterioAInventarioCompleto: (a) cada call site del grep
// canónico está listado en §B del addendum, con servicio y archivo:línea.
func TestContratoT5aCriterioAInventarioCompleto(t *testing.T) {
	adr := adrText(t)
	addendum, err := seccionAddendum(adr)
	if err != nil {
		t.Fatalf("%v", err)
	}
	inventario, err := seccionB(addendum)
	if err != nil {
		t.Fatalf("%v", err)
	}
	sitios := parseCallSites(grepCanonico(t))
	if len(sitios) == 0 {
		t.Fatal("el grep canónico no devolvió call sites parseables: no se puede verificar el criterio (a)")
	}
	if faltantes := verificarInventario(inventario, sitios); len(faltantes) > 0 {
		t.Errorf("criterio (a): el inventario §B NO lista %d call site(s) del grep canónico — inventario incompleto, la tarea FALLA: %v", len(faltantes), faltantes)
	}
}

// TestContratoT5aCriterioBFirmasGateMarcadas: (b) `@dev-security GO` y
// `Owner GO` del addendum están marcados. HOY FALLA POR DISEÑO: los marca el
// gate L4 (escalación 2026-09-14_ACC-E03-T5a-signoff.md, PENDIENTE). Un rojo
// acá NO es un bug del test — es el criterio (b) del contrato diciendo que
// T5a sigue sin firma y no puede cerrar.
func TestContratoT5aCriterioBFirmasGateMarcadas(t *testing.T) {
	adr := adrText(t)
	addendum, err := seccionAddendum(adr)
	if err != nil {
		t.Fatalf("%v", err)
	}
	firmas, err := seccionFirmasAddendum(addendum)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if faltan := verificarFirmas(firmas); len(faltan) > 0 {
		t.Errorf("criterio (b): sin firma del gate L4, faltan %v — T5a NO está aprobada (los marca el gate, no el autor del addendum)", faltan)
	}
}

// TestContratoT5aCriterioCDecisionEnContratoT5: (c) el Contrato de T5 nombra
// la opción ELEGIDA del addendum §C (cruce entre épica y ADR).
func TestContratoT5aCriterioCDecisionEnContratoT5(t *testing.T) {
	adr := adrText(t)
	addendum, err := seccionAddendum(adr)
	if err != nil {
		t.Fatalf("%v", err)
	}
	secC, err := seccionC(addendum)
	if err != nil {
		t.Fatalf("%v", err)
	}
	t5, err := bloqueT5(epicaText(t))
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := verificarDecision(t5, secC); err != nil {
		t.Errorf("criterio (c) del contrato de T5a: %v", err)
	}
}

// ------------------------------------------- negativos: el test sabe fallar

// TestNegativoT5aRechazaInventarioConFilaFaltante: (a) NEGATIVO por mutación —
// la fila de ledger-service borrada de §B debe hacer rechazar el inventario.
// Si el verificador acepta un §B amputado, el criterio (a) está muerto: un
// call site omitido pasaría como "inventario completo".
func TestNegativoT5aRechazaInventarioConFilaFaltante(t *testing.T) {
	adr := adrText(t)
	addendum, err := seccionAddendum(adr)
	if err != nil {
		t.Fatalf("%v", err)
	}
	inventario, err := seccionB(addendum)
	if err != nil {
		t.Fatalf("%v", err)
	}
	// Las filas reales de ledger-service en §B: el inventario vive en DOS
	// tablas (la clasificación canónica de 25 líneas y el detalle de call
	// sites TenantValidation §B.2) — la mutación amputará AMBAS, si no queda
	// la fila en la otra tabla y el negativo deja de reproducir el defecto.
	var filasLedger []string
	for _, l := range strings.Split(inventario, "\n") {
		if strings.Contains(l, "ledger-service") {
			filasLedger = append(filasLedger, l)
		}
	}
	if len(filasLedger) == 0 {
		t.Fatalf("no se encontró ninguna fila de ledger-service en §B para mutar: revisar estructura del addendum")
	}
	inventarioAmputado := inventario
	for _, fila := range filasLedger {
		inventarioAmputado = strings.Replace(inventarioAmputado, fila+"\n", "", 1)
	}
	sitios := parseCallSites(grepCanonico(t))
	if faltantes := verificarInventario(inventarioAmputado, sitios); len(faltantes) == 0 {
		t.Errorf("el verificador de inventario ACEPTÓ un §B sin la fila de ledger-service — un test que nunca falla no prueba nada")
	}
}

// TestNegativoT5aRechazaCallSiteNuevoNoListado: (a) NEGATIVO sintético — un
// call site que el grep devuelve y §B no lista (servicio phantom) debe ser
// detectado como inventario incompleto. Es la negativa literal del contrato:
// "si el grep devuelve un call site que el addendum no lista, la tarea falla".
func TestNegativoT5aRechazaCallSiteNuevoNoListado(t *testing.T) {
	addendum, err := seccionAddendum(adrText(t))
	if err != nil {
		t.Fatalf("%v", err)
	}
	inventario, err := seccionB(addendum)
	if err != nil {
		t.Fatalf("%v", err)
	}
	salida := "active/mercado-cercano/services/phantom-service/cmd/main.go:9:	router.Use(tenantmw.TenantValidation(tenantmw.TenantValidationConfig{"
	if faltantes := verificarInventario(inventario, parseCallSites(salida)); len(faltantes) == 0 {
		t.Errorf("el verificador de inventario ACEPTÓ un call site (phantom-service) que §B no lista — el criterio (a) no muerde")
	}
}

// TestNegativoT5aRechazaFirmasSinMarcar: (b) NEGATIVO — el addendum real con
// las firmas del gate desmarcadas (`[x]` → `[ ]`, el estado previo al gate)
// debe ser rechazado. Prueba que el verificador de firmas distingue marcado
// de no marcado y que el falso `- [x]` de la firma de T1 (otra sección) no
// contamina el chequeo.
func TestNegativoT5aRechazaFirmasSinMarcar(t *testing.T) {
	adr := adrText(t)
	addendum, err := seccionAddendum(adr)
	if err != nil {
		t.Fatalf("%v", err)
	}
	firmas, err := seccionFirmasAddendum(addendum)
	if err != nil {
		t.Fatalf("%v", err)
	}
	desmarcadas := strings.ReplaceAll(strings.ReplaceAll(firmas,
		"- [x] **@dev-security GO**", "- [ ] **@dev-security GO**"),
		"- [x] **Owner GO**", "- [ ] **Owner GO**")
	if faltan := verificarFirmas(desmarcadas); len(faltan) != 2 {
		t.Errorf("verificarFirmas sobre firmas desmarcadas devolvió %v; se esperaban los 2 GO faltantes", faltan)
	}
}

// TestNegativoT5aRechazaFirmaDeT1ComoSiFueraDelAddendum: NEGATIVO del chequeo
// GLOBAL de firmas — un addendum hipotético SIN sección de firmas no puede
// cerrar el criterio (b) gracias al `- [x]` de T1 que vive en otra parte del
// ADR. El verificador debe fallar ruidoso por sección ausente, no aprobar por
// contaminación.
func TestNegativoT5aRechazaFirmaDeT1ComoSiFueraDelAddendum(t *testing.T) {
	adr := adrText(t)
	// ADR con el addendum recortado ANTES de su sección de firmas: queda el
	// resto del ADR (incluidas las firmas de T1 con su `- [x]`) sin firmas de T5a.
	i := strings.Index(adr, "### Firmas del addendum")
	if i < 0 {
		t.Fatalf("no se pudo aislar la sección de firmas del addendum para mutar")
	}
	adrSinFirmasT5a := adr[:i]
	addendumRecortado, err := seccionAddendum(adrSinFirmasT5a)
	if err != nil {
		t.Fatalf("el addendum recortado ya no es reconocible como addendum: %v", err)
	}
	if _, err := seccionFirmasAddendum(addendumRecortado); err == nil {
		t.Errorf("seccionFirmasAddendum encontró una sección de firmas en un ADR donde el addendum no la tiene — el chequeo se contaminaría con la firma de T1")
	}
}

// TestNegativoT5aRechazaContratoT5SinDecision: (c) NEGATIVO — el Contrato de
// T5 con la decisión de flota tapada (el estado pre-T5a: el flip descrito sin
// decidir cómo verifica RS256 la flota) debe ser rechazado. También se
// rechaza un T5 que cite una opción DISTINTA de la ELEGIDA en §C.
func TestNegativoT5aRechazaContratoT5SinDecision(t *testing.T) {
	adr := adrText(t)
	addendum, err := seccionAddendum(adr)
	if err != nil {
		t.Fatalf("%v", err)
	}
	secC, err := seccionC(addendum)
	if err != nil {
		t.Fatalf("%v", err)
	}

	// Caso 1: Contrato T5 con la decisión BORRADA (texto pre-T5a).
	contratoReal := "Contrato: la credencial keyed `iam-service` pasa de `algorithm: HS256` + `secret` a `algorithm: <asimétrico>` + `rsa_public_key: ${JWT_PUBLIC_KEY}` (PEM). Aplicar exige rebuild + recreate de lab-kong"
	bloquePreT5a := "T5 · Kong verifica el algoritmo asimétrico en el borde para toda la flota\n" +
		contratoReal + "\n" +
		"Depende de: T2, T3"
	if err := verificarDecision(bloquePreT5a, secC); err == nil {
		t.Errorf("el verificador de decisión ACEPTÓ un Contrato T5 sin la decisión de flota (estado pre-T5a) — el criterio (c) no muerde")
	}

	// Caso 2: T5 cita la opción 2 mientras §C elige la 1 — ejecutaría sobre
	// el plan viejo igual que no citar nada.
	bloqueReal, err := bloqueT5(epicaText(t))
	if err != nil {
		t.Fatalf("%v", err)
	}
	lineasBloque := strings.Split(bloqueReal, "\n")
	t5OpcionEquivocada := strings.Replace(strings.Join(lineasBloque[1:], "\n"),
		"opción 1 del addendum", "opción 2 del addendum", 1)
	if err := verificarDecision(t5OpcionEquivocada, secC); err == nil {
		t.Errorf("el verificador de decisión ACEPTÓ un T5 que cita la opción 2 cuando §C elige la 1")
	}
}
