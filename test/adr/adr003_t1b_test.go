// Package adrtest vuelve ejecutables las condiciones que el gate L4 de ACC-E03 T1
// puso sobre ADR-003 — el mismo movimiento del arch_test de ACC-E01 T6: hasta T1b
// esa condición vivía en la escalación de sign-off; este test la verifica en cada
// `go test`, no cuando alguien se acuerda de re-greppear.
//
// Contrato de ACC-E03 T1b (ACC-E03-firma-asimetrica-jwks.md, ceremonia L2), tal
// como está escrito — este test verifica el CONTRATO, no el estado actual del ADR:
//
//  1. el ADR registra que rotar sin dual de borde deja en 401 a los tokens `kid1`
//     vivos (grep -ci 'kid1\|401' ≥ 1 Y el texto lo dice explícitamente);
//  2. el ADR explicita en §f la regla de selección dual
//     (grep -ci 'selección dual\|regla de selección' ≥ 1 en §f);
//  3. el "Hecho cuando" de T8 queda re-alcanceado a verificación in-process y lo
//     dice en su propio texto (bloque T8 de la épica);
//  4. NEGATIVO: grep -c 'kid' → si da 0, la condición del gate sigue abierta y la
//     tarea FALLA.
//
// Los cuatro tests TestContratoT1b_* corren contra los archivos reales. Los
// TestContratoT1b_Negativo_* mutan el texto real (borran la sección §c, tapan la
// frase de §f, etc.) y exigen que el verificador RECHACE — prueban que el test
// puede fallar. Un verificador de documento que nunca falló no prueba nada, igual
// que el arch test de la épica: es la misma tesis del "Hecho cuando" de T8(b).
//
// La condición 3 vive en la épica (management/projects/account-service/epicas/),
// fuera de este repo: se resuelve por $DEVY_PATH (contrato de ADR-001 del
// harness) con fallback a la relativa desde el module root (instalación única:
// $DEVY_PATH/platform/iam-service).
package adrtest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------- verificación

// grepCi cuenta las líneas que contienen alguno de los patrones,
// case-insensitive — el -ci literal del "Hecho cuando".
func grepCi(texto string, patrones ...string) int {
	lineas := strings.Split(texto, "\n")
	n := 0
	for _, l := range lineas {
		low := strings.ToLower(l)
		for _, p := range patrones {
			if strings.Contains(low, strings.ToLower(p)) {
				n++
				break
			}
		}
	}
	return n
}

// secciones parte el documento en bloques por header markdown (## / ###) — la
// semántica de "en §f" del contrato exige poder razonar por sección, no global.
func secciones(texto string) []string {
	var secs []string
	var actual []string
	for _, l := range strings.Split(texto, "\n") {
		if strings.HasPrefix(l, "#") {
			if len(actual) > 0 {
				secs = append(secs, strings.Join(actual, "\n"))
			}
			actual = []string{l}
		} else {
			actual = append(actual, l)
		}
	}
	if len(actual) > 0 {
		secs = append(secs, strings.Join(actual, "\n"))
	}
	return secs
}

// seccionF devuelve el bloque que empieza en "### (f)" hasta el próximo header.
// El contrato exige la regla EN §f — un match global dejaría pasar una regla de
// selección dual escrita en cualquier otra parte del ADR.
func seccionF(adr string) (string, error) {
	i := strings.Index(adr, "### (f)")
	if i < 0 {
		return "", fmt.Errorf("ADR-003: no se encontró la sección §f")
	}
	resto := adr[i:]
	if j := strings.Index(resto, "\n#"); j > 0 {
		resto = resto[:j]
	}
	return resto, nil
}

// bloqueT8 devuelve el texto de la tarea T8 de la épica hasta su "Depende de:".
// Condición 3: el re-alcance a in-process debe decirlo T8 "en su propio texto" —
// no basta que lo diga el ADR o la bitácora.
func bloqueT8(epica string) (string, error) {
	i := strings.Index(epica, "T8 ·")
	if i < 0 {
		return "", fmt.Errorf("épica ACC-E03: no se encontró el bloque de la tarea T8")
	}
	resto := epica[i:]
	if j := strings.Index(resto, "Depende de:"); j > 0 {
		resto = resto[:j]
	}
	return resto, nil
}

// verificarCostoRotacion implementa la condición 1: además del grep literal
// (kid1|401 ≥ 1), el texto tiene que DECLARAR el costo — alguna sección debe
// decir, junta, que rotar sin dual de borde deja en 401 a los tokens kid1
// vivos. "401" aparece también en §e/§f sin decir nada de rotación; exigir la
// co-ocurrencia en una misma sección es lo que separa "menciona 401" de
// "declara el costo de la rotación".
func verificarCostoRotacion(adr string) error {
	if grepCi(adr, "kid1", "401") == 0 {
		return fmt.Errorf("grep -ci 'kid1\\|401' ADR-003 == 0 (condición 1 del gate: ≥ 1)")
	}
	for _, sec := range secciones(adr) {
		low := strings.ToLower(sec)
		if strings.Contains(low, "sin dual de borde") &&
			strings.Contains(low, "kid1") &&
			strings.Contains(low, "401") {
			return nil
		}
	}
	return fmt.Errorf("ADR-003: menciona kid1 y 401 pero NINGUNA sección declara que la rotación sin dual de borde invalida los tokens kid1 vivos")
}

// verificarSeleccionDual implementa la condición 2: §f debe contener
// 'selección dual' o 'regla de selección' (grep -ci ≥ 1).
func verificarSeleccionDual(sectionF string) error {
	if grepCi(sectionF, "selección dual", "regla de selección") == 0 {
		return fmt.Errorf("§f de ADR-003 sin mención de 'selección dual' ni 'regla de selección' (condición 2 del gate)")
	}
	return nil
}

// verificarT8InProcess implementa la condición 3: el propio texto de T8 dice
// que la verificación del solapamiento es in-process.
func verificarT8InProcess(bloque string) error {
	if grepCi(bloque, "in-process") == 0 {
		return fmt.Errorf("el bloque T8 de la épica no dice en su propio texto que la verificación es in-process")
	}
	return nil
}

// verificarCondicionKid implementa la condición 4 (NEGATIVA del contrato):
// sin ninguna línea con 'kid', la condición del gate sigue abierta y la
// tarea falla. Es el análogo del grep -c 'kid' == 0.
func verificarCondicionKid(adr string) error {
	n := 0
	for _, l := range strings.Split(adr, "\n") {
		if strings.Contains(l, "kid") {
			n++
		}
	}
	if n == 0 {
		return fmt.Errorf("grep -c 'kid' ADR-003 == 0: la condición del gate L4 sigue ABIERTA y la tarea falla")
	}
	return nil
}

// ----------------------------------------------------------------------- rutas

func moduleRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}").Output()
	if err != nil {
		t.Fatalf("go list -m -f {{.Dir}}: %v", err)
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		t.Fatal("no se pudo resolver el directorio del módulo")
	}
	return root
}

// leer abre el archivo o falla ruidoso: un ADR que no existe no es "test
// salteado", es el gate L4 sin cerrar.
func leer(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no se pudo leer %s (contrato de T1b incumplible sin él): %v", path, err)
	}
	return string(b)
}

func adrText(t *testing.T) string {
	t.Helper()
	return leer(t, filepath.Join(moduleRoot(t), "docs", "adr", "ADR-003-firma-asimetrica-jwks.md"))
}

func epicaText(t *testing.T) string {
	t.Helper()
	nombre := "ACC-E03-firma-asimetrica-jwks.md"
	relativa := filepath.Join("management", "projects", "account-service", "epicas", nombre)
	if devy := os.Getenv("DEVY_PATH"); devy != "" {
		return leer(t, filepath.Join(devy, relativa))
	}
	// Fallback instalación única (ADR-001): moduleRoot = $DEVY_PATH/platform/iam-service
	// (dos niveles arriba, no tres: iam-service → platform → $DEVY_PATH)
	return leer(t, filepath.Join(moduleRoot(t), "..", "..", relativa))
}

// ------------------------------------------------- contrato: archivos reales

// TestContratoT1bCondicion1CostoRotacionDeclarado: el ADR registra el costo
// real de rotar sin dual de borde — tokens kid1 vivos → 401.
func TestContratoT1bCondicion1CostoRotacionDeclarado(t *testing.T) {
	if err := verificarCostoRotacion(adrText(t)); err != nil {
		t.Errorf("condición 1 del gate L4 incumplida: %v", err)
	}
}

// TestContratoT1bCondicion2SeleccionDualEnSeccionF: §f explicita la regla de
// selección dual (qué kid elige el verificador cuando hay dos publicados).
func TestContratoT1bCondicion2SeleccionDualEnSeccionF(t *testing.T) {
	adr := adrText(t)
	f, err := seccionF(adr)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := verificarSeleccionDual(f); err != nil {
		t.Errorf("condición 2 del gate L4 incumplida: %v", err)
	}
}

// TestContratoT1bCondicion3T8ReAlcanceadoInProcess: T8 dice en su propio texto
// que la verificación del solapamiento es in-process (no contra Kong).
func TestContratoT1bCondicion3T8ReAlcanceadoInProcess(t *testing.T) {
	epica := epicaText(t)
	t8, err := bloqueT8(epica)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := verificarT8InProcess(t8); err != nil {
		t.Errorf("condición 3 del gate L4 incumplida: %v", err)
	}
}

// TestContratoT1bCondicion4KidPresente: caso NEGATIVO del contrato — un ADR
// sin ninguna línea con 'kid' deja la condición del gate ABIERTA y la tarea
// falla. El test pasa sólo porque el ADR real menciona kid.
func TestContratoT1bCondicion4KidPresente(t *testing.T) {
	if err := verificarCondicionKid(adrText(t)); err != nil {
		t.Errorf("condición 4 (negativa) del gate L4: %v", err)
	}
}

// ------------------------------------------- negativos: el test sabe fallar

// Los cuatro tests de arriba podrían pasar trivialmente si los verificadores
// no mordieran el texto correcto. Cada mutación de acá reproduce el "control
// ausente": el texto real con la condición BORRADA/TAPADA debe ser rechazado.
// Si alguno de estos tests falla, el contrato dejó de estar blindado — el
// verificador acepta un ADR que no cumple.

// TestNegativoT1bRechazaADRSinCostoDeclarado: borrada la sección §c (donde
// vive hoy la declaración del costo), el verificador debe rechazar.
func TestNegativoT1bRechazaADRSinCostoDeclarado(t *testing.T) {
	adr := adrText(t)
	i := strings.Index(adr, "### (c)")
	j := strings.Index(adr, "### (d)")
	if i < 0 || j < 0 || j <= i {
		t.Fatalf("no se pudo aislar §c para mutar (i=%d j=%d): revisar estructura del ADR", i, j)
	}
	mutado := adr[:i] + adr[j:]
	if err := verificarCostoRotacion(mutado); err == nil {
		t.Errorf("el verificador de costo de rotación ACEPTÓ un ADR sin la declaración de §c — un test que nunca falla no prueba nada")
	}
}

// TestNegativoT1bRechazaSeleccionDualFueraDeSeccionF: la regla de selección
// dual escrita en CUALQUIER otra sección (acá simulada con la frase movida a
// §e) no cierra la condición 2 — el contrato la exige EN §f.
func TestNegativoT1bRechazaSeleccionDualFueraDeSeccionF(t *testing.T) {
	adr := adrText(t)
	f, err := seccionF(adr)
	if err != nil {
		t.Fatalf("%v", err)
	}
	// §f real con la frase tapada: ni 'selección dual' ni 'regla de selección'.
	fTapada := strings.ReplaceAll(strings.ReplaceAll(f, "selección dual", "—"), "regla de selección", "—")
	if err := verificarSeleccionDual(fTapada); err == nil {
		t.Errorf("el verificador de §f ACEPTÓ una §f sin regla de selección dual")
	}
	// Y la mera existencia global no basta: en un ADR donde la frase vive en §e
	// y §f está vacía, la condición 2 sigue abierta — el contrato la exige EN §f.
	docConFraseEnE := "## Decisión\n" +
		"### (e) Kong\nRegla de selección dual escrita en la sección equivocada\n" +
		"### (f) Cutover\nTexto de cutover: flip único, drenaje de 15 min y retiro\n"
	fEquivocada, err := seccionF(docConFraseEnE)
	if err != nil {
		t.Fatalf("seccionF del doc sintético: %v", err)
	}
	if err := verificarSeleccionDual(fEquivocada); err == nil {
		t.Errorf("regla de selección dual escrita en §e no puede cerrar la condición 2 — el contrato la exige en §f")
	}
}

// TestNegativoT1bRechazaT8SinInProcess: un T8 que no dice in-process en su
// propio texto (el texto pre-T1b: verificación contra Kong) debe ser rechazado.
func TestNegativoT1bRechazaT8SinInProcess(t *testing.T) {
	t8preT1b := "T8 · Simulacro de rotación\n" +
		"Contrato: durante el solapamiento, un token kid1 → 200 verificado CONTRA KONG\n" +
		"Hecho cuando: (a) token kid1 → 200 contra Kong, (b) tras retirar kid1 → 401\n" +
		"Depende de: T6, T7"
	if err := verificarT8InProcess(t8preT1b); err == nil {
		t.Errorf("el verificador de T8 ACEPTÓ un texto pre-T1b (verificación contra Kong) — el re-alcance a in-process no está dicho en el propio T8")
	}
}

// TestNegativoT1bGateAbiertoSiADRNoMencionaKid: ADR con toda mención de 'kid'
// tapada → grep -c 'kid' == 0 → la condición del gate sigue abierta.
func TestNegativoT1bGateAbiertoSiADRNoMencionaKid(t *testing.T) {
	adr := strings.ReplaceAll(adrText(t), "kid", "Xid")
	if err := verificarCondicionKid(adr); err == nil {
		t.Errorf("verificarCondicionKid ACEPTÓ un ADR sin ninguna línea con 'kid' — la condición del gate quedaría abierta sin que nadie falle")
	}
}