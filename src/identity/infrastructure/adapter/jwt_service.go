package adapter

import (
	"crypto/rsa"
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"

	"github.com/hornosg/iam-service/src/identity/domain/value_object"
)

// SigningKey es el material asimétrico con el que el firmador emite tokens
// de usuario (ACC-E03 T3, ADR-003 §b/§c): clave privada RSA ≥2048 + kid
// determinístico (thumbprint RFC 7638, "acc-<12hex>"). Vive en el adapter
// porque su consumidor es el firmador; config lo aliasa para el wiring
// (config importa adapter — al revés sería un ciclo de importación).
type SigningKey struct {
	PrivateKey *rsa.PrivateKey
	KID        string
}

// JWTServiceAdapter firma y verifica sólo con RS256 + kid (ADR-003). La
// aceptación dual HS256 de la ventana de cutover se retiró en ACC-E03 T6:
// el secreto simétrico compartido ya no firma ni verifica tokens de usuario.
type JWTServiceAdapter struct {
	// signingKey firma con RS256 y fija el kid del header. nil → Sign falla
	// explícito: en dev el boot no es fatal sin clave (caveat 2 de T2) y
	// asumir que existe emitiría tokens con un firmador no configurado.
	signingKey *SigningKey
	// publicKeys resuelve un token RS256 por el kid de su header (§f: la
	// selección es por header, nunca por intento y error). Contiene la pública
	// de firma más las del keyring montado (JWT_PUBLIC_KEY_DIR) — en una
	// rotación, la clave en gracia (ACC-E03 T8, ADR-003 §c).
	//
	// Inmutable tras el constructor (condición C2 de T7): Parse, PublicKeyFor
	// y el handler del JWKS la leen concurrentemente y nadie la escribe; rotar
	// es cambiar los archivos del keyring y recrear el servicio. Si algún día
	// se recarga en caliente, el swap tiene que ser atómico (un map de Go
	// leído y escrito a la vez es un fatal que gin.Recovery no atrapa).
	publicKeys map[string]*rsa.PublicKey
}

// NewJWTServiceAdapter recibe el par asimétrico de firma. signingKey puede
// ser nil en desarrollo sin clave montada: en ese estado el adapter no firma
// y rechaza todo token (keyring vacía → fail-closed).
func NewJWTServiceAdapter(signingKey *SigningKey) *JWTServiceAdapter {
	return NewJWTServiceAdapterWithKeyring(signingKey, nil)
}

// NewJWTServiceAdapterWithKeyring suma a la pública de firma las públicas del
// keyring de verificación (ACC-E03 T8, condición C1 de T7). El tipo del
// parámetro es *rsa.PublicKey: por construcción no entra material privado. Es
// la carga de claves en gracia del ciclo publicar → firmar → gracia → retirar
// (ADR-003 §c): Parse, token_revocation, authorize (vía PublicKeyFor) y el
// JWKS ven la MISMA keyring, así lo publicado es exactamente lo aceptado.
//
// Retirar una clave = sacarla del keyring y recrear: si no es la de firma,
// desaparece de publicKeys y sus tokens dan "kid desconocido o retirado".
// El map recibido se copia — mutarlo después no altera al verificador.
func NewJWTServiceAdapterWithKeyring(signingKey *SigningKey, keyring map[string]*rsa.PublicKey) *JWTServiceAdapter {
	s := &JWTServiceAdapter{
		signingKey: signingKey,
		publicKeys: make(map[string]*rsa.PublicKey, len(keyring)+1),
	}
	for kid, pub := range keyring {
		if kid != "" && pub != nil {
			s.publicKeys[kid] = pub
		}
	}
	if signingKey != nil && signingKey.PrivateKey != nil {
		s.publicKeys[signingKey.KID] = &signingKey.PrivateKey.PublicKey
	}
	return s
}

// Sign emite el token con el algoritmo de ADR-003 §a (RS256) y el kid en el
// header. El flip del firmador es atómico con el de la credencial de Kong
// (ADR-003 §f): desde T3 el login emite RS256, HS256 ya no se firma.
func (s *JWTServiceAdapter) Sign(claims *value_object.TokenClaims) (string, error) {
	if s.signingKey == nil || s.signingKey.PrivateKey == nil {
		return "", errors.New("jwt: signing key not configured — set JWT_PRIVATE_KEY_FILE (ADR-003 §b); cannot sign tokens")
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, JWTClaims{TokenClaims: *claims})
	token.Header["kid"] = s.signingKey.KID
	return token.SignedString(s.signingKey.PrivateKey)
}

// keyFunc acepta sólo RS256 con un kid de la keyring (ADR-003 §f tras el
// cutover de T6). HS256, alg none y cualquier otro método se rechazan: no
// existe ya ningún secreto simétrico con el que verificar, así que la
// confusión RS/HS no tiene rama por la que entrar.
func (s *JWTServiceAdapter) keyFunc(t *jwt.Token) (interface{}, error) {
	if t.Method.Alg() != jwt.SigningMethodRS256.Alg() {
		return nil, fmt.Errorf("método de firma no aceptado: %v", t.Header["alg"])
	}
	kid, _ := t.Header["kid"].(string)
	if kid == "" {
		return nil, errors.New("token RS256 sin kid en el header")
	}
	pub, ok := s.publicKeys[kid]
	if !ok {
		return nil, fmt.Errorf("kid desconocido o retirado: %s", kid)
	}
	return pub, nil
}

// PublicKeyFor entrega la pública RSA activa por kid para los verificadores
// que no parsean a través de este adapter (ACC-E03 T4: el gate `authorize` de
// access, que mantiene su parse MapClaims propio). Es la misma keyring que el
// keyFunc de arriba — la rotación (T8) la carga una sola vez en el constructor.
// Devuelve false si el kid es desconocido o retirado: fail-closed.
//
// ACC-E03 T4b — el receptor nil tolera el nil (forma 2 del contrato de la
// tarea). Motivo: router.go declara `var signingVerifier *adapter.JWTServiceAdapter`
// y lo pasa como PublicKeyResolver también con identity desmontado; un puntero
// nil dentro de una interfaz no es nil, el guard `publicKeys == nil` del gate
// no dispara y sin este guard acá el método desreferencia el nil → panic que
// gin.Recovery convierte en 500 por request. Se cierra EN la única función que
// puede desreferenciar (y no en el wiring) porque el fail-closed declarado del
// gate no debe depender de que cada call site futuro convierta bien su nil:
// hoy el wiring pasa el typed nil, mañana cualquier otro podría. Con el guard,
// ese estado es fail-closed igual que un kid desconocido.
func (s *JWTServiceAdapter) PublicKeyFor(kid string) (*rsa.PublicKey, bool) {
	if s == nil {
		return nil, false
	}
	pub, ok := s.publicKeys[kid]
	return pub, ok
}

// acceptedMethods es el doble control del keyFunc: WithValidMethods hace que
// el parser rechace cualquier alg distinto de RS256 ANTES de pedir la clave.
var acceptedMethods = []string{jwt.SigningMethodRS256.Alg()}

func (s *JWTServiceAdapter) Parse(tokenString string) (*value_object.TokenClaims, error) {
	jwtClaims := &JWTClaims{}
	token, err := jwt.NewParser(jwt.WithValidMethods(acceptedMethods)).ParseWithClaims(tokenString, jwtClaims, s.keyFunc)
	if err != nil {
		return nil, fmt.Errorf("error parseando token: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("token inválido")
	}
	tc := jwtClaims.TokenClaims
	return &tc, nil
}

// ParseIgnoringExpiry verifica la firma (RS256 por kid, misma regla §f) y devuelve los
// claims SIN validar expiración (v5: WithoutClaimsValidation).
// Sólo para el presenter opcional de POST /auth/refresh (ACC-E02 T8e).
func (s *JWTServiceAdapter) ParseIgnoringExpiry(tokenString string) (*value_object.TokenClaims, error) {
	jwtClaims := &JWTClaims{}
	token, err := jwt.NewParser(
		jwt.WithoutClaimsValidation(),
		jwt.WithValidMethods(acceptedMethods),
	).ParseWithClaims(tokenString, jwtClaims, s.keyFunc)
	if err != nil {
		return nil, fmt.Errorf("error parseando token (sin validar expiración): %w", err)
	}
	if !token.Valid {
		return nil, errors.New("token inválido")
	}
	tc := jwtClaims.TokenClaims
	return &tc, nil
}