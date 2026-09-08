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

// JWTServiceAdapter firma con RS256 + kid y verifica con aceptación dual
// (ADR-003 §f): durante la ventana de cutover los tokens HS256 del secreto
// viejo siguen validando in-process hasta que T6 retire la aceptación dual.
type JWTServiceAdapter struct {
	// legacySecret verifica SOLO tokens HS256 (regla de selección dual §f).
	// Jamás se usa como material de firma desde T3: el firmador es RS256.
	legacySecret string
	// signingKey firma con RS256 y fija el kid del header. nil → Sign falla
	// explícito: en dev el boot no es fatal sin clave (caveat 2 de T2) y
	// asumir que existe emitiría tokens con un firmador no configurado.
	signingKey *SigningKey
	// publicKeys resuelve un token RS256 por el kid de su header (§f: la
	// selección es por header, nunca por intento y error). Hoy una sola
	// entrada; la rotación con solapamiento (T8) agrega la clave en gracia.
	publicKeys map[string]*rsa.PublicKey
}

// NewJWTServiceAdapter recibe el secreto simétrico viejo (verificación dual
// HS256 hasta T6) y el par asimétrico de firma. signingKey puede ser nil en
// desarrollo sin clave montada: en ese estado el adapter verifica pero NO
// puede firmar.
func NewJWTServiceAdapter(legacySecret string, signingKey *SigningKey) *JWTServiceAdapter {
	s := &JWTServiceAdapter{
		legacySecret: legacySecret,
		signingKey:   signingKey,
		publicKeys:   map[string]*rsa.PublicKey{},
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

// keyFunc implementa la regla de selección dual de ADR-003 §f:
//
//	alg HS256 → sólo el secreto viejo    alg RS256 → la pública por kid
//	alg none / cualquier otro → rechazo
//
// La confusión RS/HS queda cerrada por construcción: la rama HMAC devuelve
// únicamente el secreto legacy, nunca material de la clave pública; y la
// rama RSA devuelve una *rsa.PublicKey, con la que golang-jwt sólo verifica
// firma RSA. Un token HS256 "firmado" con el PEM público como secreto cae
// en la rama HS256, se verifica contra el secreto viejo y falla.
func (s *JWTServiceAdapter) keyFunc(t *jwt.Token) (interface{}, error) {
	switch t.Method.Alg() {
	case jwt.SigningMethodHS256.Alg():
		return []byte(s.legacySecret), nil
	case jwt.SigningMethodRS256.Alg():
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("token RS256 sin kid en el header")
		}
		pub, ok := s.publicKeys[kid]
		if !ok {
			return nil, fmt.Errorf("kid desconocido o retirado: %s", kid)
		}
		return pub, nil
	default:
		return nil, fmt.Errorf("método de firma no aceptado: %v", t.Header["alg"])
	}
}

// dualAcceptedMethods es el doble control del keyFunc: WithValidMethods hace
// que el parser rechace alg fuera de {HS256, RS256} ANTES de pedir la clave.
var dualAcceptedMethods = []string{jwt.SigningMethodHS256.Alg(), jwt.SigningMethodRS256.Alg()}

func (s *JWTServiceAdapter) Parse(tokenString string) (*value_object.TokenClaims, error) {
	jwtClaims := &JWTClaims{}
	token, err := jwt.NewParser(jwt.WithValidMethods(dualAcceptedMethods)).ParseWithClaims(tokenString, jwtClaims, s.keyFunc)
	if err != nil {
		return nil, fmt.Errorf("error parseando token: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("token inválido")
	}
	tc := jwtClaims.TokenClaims
	return &tc, nil
}

// ParseIgnoringExpiry verifica la firma (dual, misma regla §f) y devuelve los
// claims SIN validar expiración (v5: WithoutClaimsValidation).
// Sólo para el presenter opcional de POST /auth/refresh (ACC-E02 T8e).
func (s *JWTServiceAdapter) ParseIgnoringExpiry(tokenString string) (*value_object.TokenClaims, error) {
	jwtClaims := &JWTClaims{}
	token, err := jwt.NewParser(
		jwt.WithoutClaimsValidation(),
		jwt.WithValidMethods(dualAcceptedMethods),
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