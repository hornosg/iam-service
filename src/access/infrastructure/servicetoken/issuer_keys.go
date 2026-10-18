package servicetoken

import (
	"crypto/rsa"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/hornosg/go-shared/infrastructure/jwtkeyring"
	"github.com/hornosg/iam-service/src/access/domain/port"
)

// issuerDirName es la misma regla de nombre que s2s-keygen.sh.
var issuerDirName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// maxKeysPerIssuer: hasta dos claves por emisor, para rotar (ADR-006 «Registro»).
const maxKeysPerIssuer = 2

// IssuerKeys es el registro de públicas de emisores, indexado por emisor y después por kid.
// Vive separado del keyring de verificación de iam: nunca se mezcla con JWT_PUBLIC_KEY_DIR ni se
// publica en el JWKS (AssertDisjoint lo verifica al arranque).
type IssuerKeys struct {
	byIssuer map[string]map[string]*rsa.PublicKey
}

var _ port.IssuerKeyRegistry = (*IssuerKeys)(nil)

// LoadIssuerKeys lee <root>/<servicio>/*.pem (sólo PUBLIC KEY, RSA ≥ 2048, kid derivado del
// material: mismas reglas que jwtkeyring.Load, que rechaza un bloque privado). root vacío → registro
// vacío: ningún emisor puede autenticarse (fail-closed), que es el estado hasta montar el primero.
func LoadIssuerKeys(root string) (*IssuerKeys, error) {
	reg := &IssuerKeys{byIssuer: map[string]map[string]*rsa.PublicKey{}}
	if root == "" {
		return reg, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("s2s issuer keys: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue // archivos sueltos y carpetas ocultas (.git) no son emisores
		}
		if !issuerDirName.MatchString(e.Name()) {
			return nil, fmt.Errorf("s2s issuer keys: nombre de carpeta de emisor inesperado %q", e.Name())
		}
		ring, err := jwtkeyring.Load(filepath.Join(root, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("s2s issuer keys: emisor %s: %w", e.Name(), err)
		}
		kids := ring.KIDs()
		if len(kids) > maxKeysPerIssuer {
			return nil, fmt.Errorf("s2s issuer keys: emisor %s tiene %d claves (máximo %d)", e.Name(), len(kids), maxKeysPerIssuer)
		}
		keys := make(map[string]*rsa.PublicKey, len(kids))
		for _, kid := range kids {
			pub, _ := ring.PublicKeyFor(kid)
			keys[kid] = pub
		}
		reg.byIssuer[e.Name()] = keys
	}
	return reg, nil
}

func (r *IssuerKeys) PublicKey(issuer, kid string) (*rsa.PublicKey, bool) {
	if r == nil {
		return nil, false
	}
	pub, ok := r.byIssuer[issuer][kid]
	return pub, ok
}

// KIDs devuelve todos los kid registrados, ordenados (para AssertDisjoint y tests).
func (r *IssuerKeys) KIDs() []string {
	var out []string
	if r == nil {
		return out
	}
	for _, keys := range r.byIssuer {
		for kid := range keys {
			out = append(out, kid)
		}
	}
	sort.Strings(out)
	return out
}

// AssertDisjoint falla si una pública de emisor aparece en el keyring de verificación de iam
// (y por lo tanto en el JWKS): ese emisor podría firmar tokens de usuario o de servicio con su
// propia privada. Se llama al arranque, antes de montar la ruta de emisión.
func AssertDisjoint(issuers *IssuerKeys, verificationKIDs []string) error {
	own := make(map[string]struct{}, len(verificationKIDs))
	for _, kid := range verificationKIDs {
		own[kid] = struct{}{}
	}
	for _, kid := range issuers.KIDs() {
		if _, clash := own[kid]; clash {
			return fmt.Errorf("s2s issuer keys: el kid %s de un emisor está en el keyring de verificación de iam", kid)
		}
	}
	return nil
}
