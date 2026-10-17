package adapter

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"math/big"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/hornosg/iam-service/src/identity/domain/value_object"
)

// ACC-E03 T7 — contrato: el JWKS "incluye las claves en gracia durante una
// rotación". Hoy no hay API pública para cargar una clave en gracia (llega con
// T8), así que la keyring se arma en white-box: lo que se prueba es que
// PublicJWKS publica TODA la keyring del verificador, no sólo la de firma.

func genKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	return k
}

func jwkPublicKey(t *testing.T, j JWK) *rsa.PublicKey {
	t.Helper()
	nb, err := base64.RawURLEncoding.DecodeString(j.N)
	if err != nil {
		t.Fatalf("n no es base64url sin padding: %v", err)
	}
	eb, err := base64.RawURLEncoding.DecodeString(j.E)
	if err != nil {
		t.Fatalf("e no es base64url sin padding: %v", err)
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: int(new(big.Int).SetBytes(eb).Int64())}
}

// Durante el solapamiento la keyring tiene la clave nueva (firma) y la vieja
// (gracia): las dos salen en el JWKS, cada una con su kid, y un token firmado
// con la vieja se verifica con la entrada publicada para su kid.
func TestPublicJWKS_IncluyeLaClaveEnGracia(t *testing.T) {
	vieja, nueva := genKey(t), genKey(t)
	const kidVieja, kidNueva = "acc-000000000old", "acc-000000000new"
	svc := NewJWTServiceAdapter(&SigningKey{PrivateKey: nueva, KID: kidNueva})
	svc.publicKeys[kidVieja] = &vieja.PublicKey

	doc := svc.PublicJWKS()

	if len(doc.Keys) != 2 {
		t.Fatalf("con una clave en gracia el JWKS debe publicar 2 claves, publicó %d", len(doc.Keys))
	}
	porKid := map[string]JWK{}
	for _, k := range doc.Keys {
		porKid[k.Kid] = k
	}
	if !jwkPublicKey(t, porKid[kidNueva]).Equal(&nueva.PublicKey) {
		t.Errorf("la entrada %s no es la pública de la clave de firma", kidNueva)
	}
	if !jwkPublicKey(t, porKid[kidVieja]).Equal(&vieja.PublicKey) {
		t.Errorf("la entrada %s no es la pública de la clave en gracia", kidVieja)
	}

	// Un token aún vivo firmado con la vieja verifica con lo publicado.
	firmadorViejo := NewJWTServiceAdapter(&SigningKey{PrivateKey: vieja, KID: kidVieja})
	tok, err := firmadorViejo.Sign(&value_object.TokenClaims{UserID: uuid.New()})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	_, err = jwt.Parse(tok, func(jt *jwt.Token) (interface{}, error) {
		kid, _ := jt.Header["kid"].(string)
		return jwkPublicKey(t, porKid[kid]), nil
	}, jwt.WithValidMethods([]string{"RS256"}))
	if err != nil {
		t.Errorf("un consumidor con sólo el JWKS no verifica el token de la clave en gracia: %v", err)
	}
}

// NEGATIVA: una clave retirada de la keyring no se sigue publicando — lo
// publicado es exactamente lo aceptado. Si el JWKS cacheara/arrastrara kids
// retirados, un consumidor fuera de Kong seguiría aceptando la clave vieja.
func TestPublicJWKS_NoPublicaUnaClaveRetirada(t *testing.T) {
	vieja, nueva := genKey(t), genKey(t)
	const kidVieja, kidNueva = "acc-000000000old", "acc-000000000new"
	svc := NewJWTServiceAdapter(&SigningKey{PrivateKey: nueva, KID: kidNueva})
	svc.publicKeys[kidVieja] = &vieja.PublicKey
	if len(svc.PublicJWKS().Keys) != 2 {
		t.Fatal("precondición: la clave en gracia debe estar publicada antes de retirarla")
	}

	delete(svc.publicKeys, kidVieja)
	doc := svc.PublicJWKS()

	if len(doc.Keys) != 1 || doc.Keys[0].Kid != kidNueva {
		t.Fatalf("tras retirar %s el JWKS debe tener sólo %s, tiene %+v", kidVieja, kidNueva, doc.Keys)
	}
	// y lo que el verificador rechaza coincide con lo que dejó de publicar.
	if _, ok := svc.PublicKeyFor(kidVieja); ok {
		t.Errorf("PublicKeyFor sigue aceptando el kid retirado")
	}
}

// Orden estable por kid: dos llamadas sobre la misma keyring dan el mismo
// documento (un consumidor que compara/cachea el JWKS no ve "cambios" falsos).
func TestPublicJWKS_OrdenEstablePorKid(t *testing.T) {
	svc := NewJWTServiceAdapter(&SigningKey{PrivateKey: genKey(t), KID: "acc-bbbbbbbbbbbb"})
	svc.publicKeys["acc-aaaaaaaaaaaa"] = &genKey(t).PublicKey
	svc.publicKeys["acc-cccccccccccc"] = &genKey(t).PublicKey

	for i := 0; i < 20; i++ {
		doc := svc.PublicJWKS()
		got := []string{doc.Keys[0].Kid, doc.Keys[1].Kid, doc.Keys[2].Kid}
		want := []string{"acc-aaaaaaaaaaaa", "acc-bbbbbbbbbbbb", "acc-cccccccccccc"}
		for j := range want {
			if got[j] != want[j] {
				t.Fatalf("orden no estable: %v, se esperaba %v", got, want)
			}
		}
	}
}
