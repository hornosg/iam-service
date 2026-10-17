// ACC-E03 T8 — casos de QA sobre el simulacro de rotación, escritos contra el
// contrato de la épica (no contra la implementación). Complementan
// rotation_t8_test.go con lo que el simulacro feliz no ejercita:
//
//   - (b) NEGATIVO por re-etiquetado: un token firmado con la privada de kid1
//     retirada al que se le cambia el header a kid=kid2 no puede pasar. Si el
//     verificador probara claves por intento y error, o eligiera por kid sin
//     verificar la firma con ESA clave, la rotación no revocaría nada.
//   - (b) NEGATIVO sin kid: un token kid1 sin header kid tras retirar → 401.
//   - C2 de T7: el keyring entregado al constructor se copia; mutarlo después
//     (retirar o agregar en caliente) no cambia lo aceptado ni lo publicado.
//   - C1 de T7: la carga de claves en gracia falla cerrado ante config rota
//     (directorio inexistente, clave débil) en vez de arrancar con keyring vacía.
package rotation_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hornosg/iam-service/src/identity/infrastructure/adapter"
	"github.com/hornosg/iam-service/src/identity/infrastructure/config"
)

// relabel re-firma los claims de un token con `signer` pero declarando `kid`
// en el header (kid vacío = sin header kid). Simula a quien conserva la
// privada de kid1 retirada y quiere colarla como kid2.
func relabel(t *testing.T, token string, signer *rsa.PrivateKey, kid string) string {
	t.Helper()
	claims := jwt.MapClaims{}
	_, _, err := jwt.NewParser().ParseUnverified(token, claims)
	require.NoError(t, err)
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	if kid != "" {
		tok.Header["kid"] = kid
	}
	s, err := tok.SignedString(signer)
	require.NoError(t, err)
	return s
}

// Control positivo del helper: re-firmar con la clave CORRECTA para el kid
// declarado tiene que validar; si no, los negativos de abajo pasarían por un
// helper roto y no por el verificador.
func TestRotacion_T8_QA_RelabelControlPositivo(t *testing.T) {
	kid2 := config.SigningKeyKID(&key2.PublicKey)
	d := deploy(t, key2, keyringDir(t, key2))
	tok := relabel(t, d.login(t), key2, kid2)

	parseOK, me, users := d.accepts(t, tok)
	assert.True(t, parseOK)
	assert.Equal(t, http.StatusOK, me)
	assert.Equal(t, http.StatusOK, users)
}

func TestRotacion_T8_QA_Kid1RetiradoReetiquetadoComoKid2_Rechazado(t *testing.T) {
	kid2 := config.SigningKeyKID(&key2.PublicKey)
	dirRetirado := keyringDir(t, key2)
	d := deploy(t, key2, dirRetirado)

	// Claims válidos (de un login real), firmados con la privada de kid1
	// retirada pero declarando kid2.
	forged := relabel(t, d.login(t), key1, kid2)

	parseOK, me, users := d.accepts(t, forged)
	assert.False(t, parseOK, "NEGATIVO: Parse aceptó una firma de kid1 retirado declarada como kid2")
	assert.Equal(t, http.StatusUnauthorized, me, "NEGATIVO: token_revocation aceptó kid1 re-etiquetado")
	assert.Equal(t, http.StatusUnauthorized, users, "NEGATIVO: authorize aceptó kid1 re-etiquetado")
	assert.False(t, fleetAccepts(t, dirRetirado, forged), "NEGATIVO: la flota aceptó kid1 re-etiquetado")
}

// En gracia ambas claves están publicadas; aun así una firma de kid1 declarada
// como kid2 no vale: la selección es por header Y la firma se verifica con la
// clave de ese kid (ADR-003 §f), no con "cualquiera del keyring".
func TestRotacion_T8_QA_EnGraciaKidCruzado_Rechazado(t *testing.T) {
	kid2 := config.SigningKeyKID(&key2.PublicKey)
	dirGracia := keyringDir(t, key1, key2)
	d := deploy(t, key2, dirGracia)

	crossed := relabel(t, d.login(t), key1, kid2)

	parseOK, me, users := d.accepts(t, crossed)
	assert.False(t, parseOK, "kid cruzado en gracia: la firma no corresponde al kid declarado")
	assert.Equal(t, http.StatusUnauthorized, me)
	assert.Equal(t, http.StatusUnauthorized, users)
	assert.False(t, fleetAccepts(t, dirGracia, crossed))
}

func TestRotacion_T8_QA_Kid1RetiradoSinHeaderKid_Rechazado(t *testing.T) {
	dirRetirado := keyringDir(t, key2)
	d := deploy(t, key2, dirRetirado)

	noKid := relabel(t, d.login(t), key1, "")

	parseOK, me, users := d.accepts(t, noKid)
	assert.False(t, parseOK, "NEGATIVO: un token sin kid firmado con kid1 retirado no puede validar")
	assert.Equal(t, http.StatusUnauthorized, me)
	assert.Equal(t, http.StatusUnauthorized, users)
	assert.False(t, fleetAccepts(t, dirRetirado, noKid))
}

// C2: el verificador no comparte el map con quien lo construyó. Si lo
// compartiera, retirar "en caliente" sería posible (y racy), y agregar una
// clave al map ajeno la haría aceptada sin pasar por la carga validada.
func TestRotacion_T8_QA_KeyringCopiadoInmutable(t *testing.T) {
	kid1 := config.SigningKeyKID(&key1.PublicKey)
	kid2 := config.SigningKeyKID(&key2.PublicKey)
	key3 := mustKey()
	kid3 := config.SigningKeyKID(&key3.PublicKey)

	// Firma kid2, gracia kid1.
	ring := map[string]*rsa.PublicKey{kid1: &key1.PublicKey, kid2: &key2.PublicKey}
	verifier := adapter.NewJWTServiceAdapterWithKeyring(signingKey(key2), ring)

	tok1 := deploy(t, key1, keyringDir(t, key1)).login(t)
	tok3 := deploy(t, key3, keyringDir(t, key3)).login(t)

	delete(ring, kid1)
	ring[kid3] = &key3.PublicKey

	_, err := verifier.Parse(tok1)
	assert.NoError(t, err, "C2: borrar kid1 del map de origen no puede retirarlo del verificador ya construido")
	_, ok := verifier.PublicKeyFor(kid1)
	assert.True(t, ok, "C2: PublicKeyFor debe seguir resolviendo kid1")

	_, err = verifier.Parse(tok3)
	assert.Error(t, err, "C2: agregar kid3 al map de origen no puede hacerlo aceptado")
	_, ok = verifier.PublicKeyFor(kid3)
	assert.False(t, ok, "C2: kid3 no puede resolverse")
}

// Entradas basura en el keyring (kid vacío o pública nil) no pueden terminar
// aceptando tokens sin kid ni provocar un panic al verificar.
func TestRotacion_T8_QA_KeyringConEntradasInvalidas_SeIgnoran(t *testing.T) {
	kid2 := config.SigningKeyKID(&key2.PublicKey)
	ring := map[string]*rsa.PublicKey{"": &key1.PublicKey, "kid-nil": nil}
	verifier := adapter.NewJWTServiceAdapterWithKeyring(signingKey(key2), ring)

	tok2 := deploy(t, key2, keyringDir(t, key2)).login(t)
	noKid := relabel(t, tok2, key1, "")
	nilKid := relabel(t, tok2, key1, "kid-nil")

	_, err := verifier.Parse(noKid)
	assert.Error(t, err, "una entrada con kid vacío no puede aceptar tokens sin kid")
	assert.NotPanics(t, func() { _, err = verifier.Parse(nilKid) })
	assert.Error(t, err, "una entrada con pública nil no puede aceptar nada")
	_, ok := verifier.PublicKeyFor("kid-nil")
	assert.False(t, ok)

	_, err = verifier.Parse(tok2)
	assert.NoError(t, err, "control: la clave de firma sigue validando")
	_, ok = verifier.PublicKeyFor(kid2)
	assert.True(t, ok)
}

// C1: un JWT_PUBLIC_KEY_DIR configurado pero inexistente es config rota. Si
// cargara como keyring vacía, el retiro "funcionaría" por accidente y la
// gracia se perdería en silencio (tokens kid1 vivos → 401 sin aviso).
func TestRotacion_T8_QA_KeyringDirInexistente_Error(t *testing.T) {
	t.Setenv("JWT_PUBLIC_KEY_DIR", filepath.Join(t.TempDir(), "no-existe"))
	ring, err := config.LoadVerificationKeyringFromEnv()
	assert.Error(t, err)
	assert.Nil(t, ring)
}

// C1: una clave débil en el keyring no puede entrar como clave en gracia.
func TestRotacion_T8_QA_KeyringConClaveDebil_Error(t *testing.T) {
	weak, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)
	dir := keyringDir(t, key1)
	der, err := x509.MarshalPKIXPublicKey(&weak.PublicKey)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "weak.pem"),
		pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), 0o644))

	t.Setenv("JWT_PUBLIC_KEY_DIR", dir)
	_, err = config.LoadVerificationKeyringFromEnv()
	assert.Error(t, err, "una RSA de 1024 bits no puede cargarse como clave en gracia")
}

// Lo cargado desde disco es exactamente lo publicado: el keyring cargado por
// config tiene los kids derivados del material (no del nombre de archivo).
func TestRotacion_T8_QA_KidDerivadoDelMaterialNoDelArchivo(t *testing.T) {
	kid1 := config.SigningKeyKID(&key1.PublicKey)
	dir := t.TempDir()
	der, err := x509.MarshalPKIXPublicKey(&key1.PublicKey)
	require.NoError(t, err)
	// Nombre de archivo engañoso: dice kid2.
	misleading := config.SigningKeyKID(&key2.PublicKey) + ".pem"
	require.NoError(t, os.WriteFile(filepath.Join(dir, misleading),
		pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), 0o644))

	t.Setenv("JWT_PUBLIC_KEY_DIR", dir)
	ring, err := config.LoadVerificationKeyringFromEnv()
	require.NoError(t, err)
	keys := make([]string, 0, len(ring))
	for k := range ring {
		keys = append(keys, k)
	}
	assert.Equal(t, []string{kid1}, keys,
		"el kid sale del material; un nombre de archivo '%s' no puede publicar kid2", strings.TrimSuffix(misleading, ".pem"))
}
