package servicetoken

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hornosg/iam-service/src/access/domain/exception"
)

// O-2 (review L4 de T2): cada emisión rechazada deja un evento de seguridad con el motivo, el sub
// ya verificado y lo pedido; nunca la aserción.
func TestIssue_RechazosDejanEventoDeSeguridad(t *testing.T) {
	f := newFixture(t, nil)
	ctx := context.Background()

	// 401 sin aserción: no hay sub verificado.
	if _, err := f.uc.Issue(ctx, cmd("", "stock-service", "stock:read")); !errors.Is(err, exception.ErrServiceTokenUnauthenticated) {
		t.Fatalf("got %v", err)
	}
	// 403: firma válida pidiendo un aud fuera de su registro (la señal de un emisor comprometido).
	raw := f.assertion(t, nil)
	if _, err := f.uc.Issue(ctx, cmd(raw, "pim-service-que-no-existe", "pim:admin")); !errors.Is(err, exception.ErrServiceTokenForbidden) {
		t.Fatalf("got %v", err)
	}
	// 503: almacén anti-replay caído, con la firma ya verificada.
	f.replay.err = errors.New("postgres caído")
	if _, err := f.uc.Issue(ctx, cmd(f.assertion(t, nil), "stock-service", "stock:read")); !errors.Is(err, exception.ErrServiceTokenUnavailable) {
		t.Fatalf("got %v", err)
	}

	if len(f.auditor.rejected) != 3 {
		t.Fatalf("se esperaban 3 eventos de rechazo, hay %d: %+v", len(f.auditor.rejected), f.auditor.rejected)
	}
	want := []struct{ reason, sub string }{
		{"invalid_client", ""},
		{"invalid_scope", "catalog-bff-service"},
		{"temporarily_unavailable", "catalog-bff-service"},
	}
	for i, w := range want {
		got := f.auditor.rejected[i]
		if got.Reason != w.reason || got.Subject != w.sub {
			t.Errorf("evento %d: %+v, se esperaba reason=%s sub=%q", i, got, w.reason, w.sub)
		}
		if strings.Contains(got.Audience+got.Scope+got.Reason+got.Subject, raw) {
			t.Errorf("evento %d filtra la aserción", i)
		}
	}
	if got := f.auditor.rejected[1]; got.Audience != "pim-service-que-no-existe" || got.Scope != "pim:admin" {
		t.Errorf("el evento 403 debe registrar lo pedido: %+v", got)
	}
	if len(f.auditor.calls) != 0 {
		t.Errorf("un rechazo no es una emisión: %d", len(f.auditor.calls))
	}
}

type countingLimiter struct{ inner *WindowLimiter }

func (c countingLimiter) Allow(k string) bool { return c.inner.Allow(k) }

// N-1: repetir una aserción capturada (jti ya visto) no consume el cupo del emisor víctima.
func TestIssue_RepeticionDeAserciónNoConsumeElCupoDelEmisor(t *testing.T) {
	f := newFixture(t, nil)
	f.uc.WithIssuerLimiter(countingLimiter{NewWindowLimiter(2, time.Minute, func() time.Time { return f.now })})
	ctx := context.Background()

	first := f.assertion(t, nil)
	if _, err := f.uc.Issue(ctx, cmd(first, "stock-service", "stock:read")); err != nil {
		t.Fatalf("primera emisión: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := f.uc.Issue(ctx, cmd(first, "stock-service", "stock:read")); !errors.Is(err, exception.ErrServiceTokenUnauthenticated) {
			t.Fatalf("repetición %d: se esperaba 401 por jti repetido, got %v", i, err)
		}
	}
	// El cupo de 2 sólo gastó 1: una aserción nueva todavía pasa.
	if _, err := f.uc.Issue(ctx, cmd(f.assertion(t, nil), "stock-service", "stock:read")); err != nil {
		t.Fatalf("las repeticiones consumieron el cupo del emisor: %v", err)
	}
	// Y la tercera aserción legítima sí agota el cupo (429).
	if _, err := f.uc.Issue(ctx, cmd(f.assertion(t, nil), "stock-service", "stock:read")); !errors.Is(err, exception.ErrServiceTokenRateLimited) {
		t.Fatalf("se esperaba 429 al agotar el cupo, got %v", err)
	}
}

// N-7: el cargador ignora carpetas ocultas (.git) y falla ante un nombre de emisor inesperado.
func TestLoadIssuerKeys_FiltraNombresDeCarpeta(t *testing.T) {
	keys(t)
	root := t.TempDir()
	writePub(t, filepath.Join(root, "catalog-bff-service"), &bffKey.PublicKey)
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	reg, err := LoadIssuerKeys(root)
	if err != nil {
		t.Fatalf(".git no debe romper el arranque: %v", err)
	}
	if len(reg.KIDs()) != 1 {
		t.Fatalf("sólo catalog-bff-service es emisor: %v", reg.KIDs())
	}
	if err := os.MkdirAll(filepath.Join(root, "Raro_Nombre"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadIssuerKeys(root); err == nil {
		t.Fatal("un nombre de emisor fuera de la regla debe fallar el arranque")
	}
}
