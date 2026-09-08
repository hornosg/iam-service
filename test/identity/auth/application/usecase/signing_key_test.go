package usecase_test

import (
	"crypto/rand"
	"crypto/rsa"
	"sync"

	"github.com/hornosg/iam-service/src/identity/infrastructure/adapter"
	"github.com/hornosg/iam-service/src/identity/infrastructure/config"
)

// ACC-E03 T3: desde el flip del firmador, Sign es RS256+kid, así que todo test
// que ejerce el adapter real necesita material asimétrico. Se genera UNA sola
// clave por paquete (sync.Once): RSA-2048 cuesta ~100ms y 19 call sites no
// tienen por qué pagarla cada uno. El kid se deriva con la MISMA función del
// runtime (config.SigningKeyKID), no con un literal, para que estos tests
// sigan pasando si la derivación cambia.
var (
	testSigningKeyOnce sync.Once
	testSigningKey     *adapter.SigningKey
)

func newTestSigningKey() *adapter.SigningKey {
	testSigningKeyOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		testSigningKey = &adapter.SigningKey{
			PrivateKey: key,
			KID:        config.SigningKeyKID(&key.PublicKey),
		}
	})
	return testSigningKey
}