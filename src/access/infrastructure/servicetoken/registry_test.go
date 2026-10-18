package servicetoken

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hornosg/go-shared/infrastructure/jwtkeyring"
	"github.com/hornosg/iam-service/src/access/domain/entity"
)

func TestPoliticaVersionada_SoloCatalogBFFYSinScopesAmplios(t *testing.T) {
	p, err := NewStaticPolicies()
	if err != nil {
		t.Fatalf("la política versionada debe validar: %v", err)
	}
	if len(p.byService) != 1 {
		t.Fatalf("T2 registra sólo a catalog-bff, hay %d emisores", len(p.byService))
	}
	bff, ok := p.Policy("catalog-bff-service")
	if !ok || bff.Namespace != "mc" {
		t.Fatalf("catalog-bff-service debe estar en el namespace mc: %+v", bff)
	}
}

func TestPolitica_RechazaScopesProhibidosYComodines(t *testing.T) {
	for _, scope := range []string{"system:admin", "tenant:admin", "stock:*", "*", "sinaccion"} {
		_, err := newStaticPolicies(map[string][]entity.IssuerPolicy{"mc": {{
			Service:   "x-service",
			Audiences: map[string]entity.AudiencePolicy{"stock-service": {Scopes: map[string]entity.ScopeClass{scope: entity.ScopeClassSystem}}},
		}}})
		if err == nil {
			t.Fatalf("la política debe rechazar el scope %q", scope)
		}
	}
}

func TestLoadIssuerKeys_RechazaPrivadaYMasDeDosClaves(t *testing.T) {
	keys(t)
	root := t.TempDir()
	dir := filepath.Join(root, "catalog-bff-service")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	priv := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(bffKey)})
	if err := os.WriteFile(filepath.Join(dir, "k.pem"), priv, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadIssuerKeys(root); err == nil {
		t.Fatal("una privada en el registro de emisores debe impedir el arranque")
	}

	root = t.TempDir()
	dir = filepath.Join(root, "catalog-bff-service")
	for i := 0; i < 3; i++ {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		writePub(t, dir, &k.PublicKey)
	}
	if _, err := LoadIssuerKeys(root); err == nil || !strings.Contains(err.Error(), "máximo") {
		t.Fatalf("más de dos claves por emisor debe fallar, got %v", err)
	}
}

func TestLoadIssuerKeys_SinRaizEsRegistroVacio(t *testing.T) {
	reg, err := LoadIssuerKeys("")
	if err != nil || len(reg.KIDs()) != 0 {
		t.Fatalf("sin raíz: registro vacío fail-closed, got %v %v", reg.KIDs(), err)
	}
	if _, ok := reg.PublicKey("catalog-bff-service", "x"); ok {
		t.Fatal("registro vacío no resuelve ninguna clave")
	}
}

func TestAssertDisjoint_DetectaClaveDeEmisorEnKeyringDeIAM(t *testing.T) {
	keys(t)
	root := t.TempDir()
	writePub(t, filepath.Join(root, "catalog-bff-service"), &bffKey.PublicKey)
	reg, err := LoadIssuerKeys(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := AssertDisjoint(reg, []string{jwtkeyring.KID(&bffKey.PublicKey)}); err == nil {
		t.Fatal("una pública de emisor en el keyring de verificación debe impedir el arranque")
	}
	if err := AssertDisjoint(reg, []string{jwtkeyring.KID(&iamKey.PublicKey)}); err != nil {
		t.Fatalf("keyrings disjuntos: %v", err)
	}
}
