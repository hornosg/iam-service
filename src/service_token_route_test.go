package main

// ACC-E09 T2 — emisión del token de servicio por HTTP, con el router real (buildRouter): el
// endpoint queda fuera de TenantValidation y de los gates de usuario, mapea cada error del dominio
// a un único código, el token emitido se valida con las claves que publica el JWKS de ACC-E03 T7
// y ningún kid del registro de emisores aparece en el JWKS.

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/hornosg/go-shared/infrastructure/jwtkeyring"
	"github.com/hornosg/go-shared/infrastructure/s2stoken"

	accessconfig "github.com/hornosg/iam-service/src/access/infrastructure/config"
	identityconfig "github.com/hornosg/iam-service/src/identity/infrastructure/config"
)

const s2sIssuer = "catalog-bff-service"

func genKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func writePEM(t *testing.T, dir, name string, block *pem.Block) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), pem.EncodeToMemory(block), 0o644); err != nil {
		t.Fatal(err)
	}
}

func pubBlock(t *testing.T, pub *rsa.PublicKey) *pem.Block {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return &pem.Block{Type: "PUBLIC KEY", Bytes: der}
}

func privBlock(t *testing.T, k *rsa.PrivateKey) *pem.Block {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return &pem.Block{Type: "PRIVATE KEY", Bytes: der}
}

type s2sRoute struct {
	r         *gin.Engine
	mock      sqlmock.Sqlmock
	bffKey    *rsa.PrivateKey
	issuerKID string
}

// s2sRouter arma el router real con la clave de firma de iam, el registro de emisores con la
// pública de catalog-bff (emisor de prueba, nunca la clave del iam vivo) y appDB sobre sqlmock
// para el almacén de jti.
func s2sRouter(t *testing.T) *s2sRoute {
	t.Helper()
	jwksRouter(t) // fija el entorno de firma (JWT_PRIVATE_KEY inline, debug)
	bff := genKey(t)
	root := t.TempDir()
	writePEM(t, filepath.Join(root, s2sIssuer), jwtkeyring.KID(&bff.PublicKey)+".pem", pubBlock(t, &bff.PublicKey))
	t.Setenv("S2S_ISSUER_KEYS_DIR", root)

	appDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	mock.MatchExpectationsInOrder(false)
	t.Cleanup(func() { appDB.Close() })
	loginDB, err := sql.Open("postgres", "postgres://dummy:dummy@127.0.0.1:1/dummy?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { loginDB.Close() })
	return &s2sRoute{r: buildRouter(appDB, loginDB), mock: mock, bffKey: bff, issuerKID: jwtkeyring.KID(&bff.PublicKey)}
}

func (s *s2sRoute) assertion(t *testing.T, key *rsa.PrivateKey, jti string) string {
	t.Helper()
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": s2sIssuer, "sub": s2sIssuer, "aud": s2stoken.AssertionAudience,
		"iat": now.Unix(), "exp": now.Add(30 * time.Second).Unix(), "jti": jti,
	})
	tok.Header["kid"] = jwtkeyring.KID(&key.PublicKey)
	if key == s.bffKey {
		tok.Header["kid"] = s.issuerKID
	}
	signed, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func (s *s2sRoute) post(form url.Values, ip string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/s2s/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = ip + ":40000"
	w := httptest.NewRecorder()
	s.r.ServeHTTP(w, req)
	return w
}

func tokenForm(assertion, aud, scope string) url.Values {
	return url.Values{
		"grant_type":            {"client_credentials"},
		"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
		"client_assertion":      {assertion},
		"audience":              {aud},
		"scope":                 {scope},
	}
}

func (s *s2sRoute) expectJTI(rows int64, err error) {
	e := s.mock.ExpectExec(`INSERT INTO s2s_assertion_jti`)
	if err != nil {
		e.WillReturnError(err)
		return
	}
	e.WillReturnResult(sqlmock.NewResult(0, rows))
}

// keyringFromJWKS reconstruye un keyring sólo con lo que publica GET /.well-known/jwks.json.
func keyringFromJWKS(t *testing.T, r *gin.Engine) (*jwtkeyring.Keyring, []string) {
	t.Helper()
	w := serve(r, http.MethodGet, "/.well-known/jwks.json")
	if w.Code != http.StatusOK {
		t.Fatalf("JWKS → %d", w.Code)
	}
	var doc struct {
		Keys []struct{ Kid, N, E string } `json:"keys"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var kids []string
	for _, k := range doc.Keys {
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if err1 != nil || err2 != nil {
			t.Fatalf("JWK mal codificado: %v %v", err1, err2)
		}
		pub := &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
		writePEM(t, dir, k.Kid+".pem", pubBlock(t, pub))
		kids = append(kids, k.Kid)
	}
	ring, err := jwtkeyring.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return ring, kids
}

func TestServiceTokenRoute_EmiteYElJWKSLoValida(t *testing.T) {
	s := s2sRouter(t)
	s.expectJTI(1, nil)
	w := s.post(tokenForm(s.assertion(t, s.bffKey, uuid.NewString()), "stock-service", "stock:read"), "10.0.0.1")
	if w.Code != http.StatusOK {
		t.Fatalf("emisión → %d %s, se esperaba 200", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q, se esperaba no-store", w.Header().Get("Cache-Control"))
	}
	var body struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TokenType != "service" || body.ExpiresIn != 300 {
		t.Errorf("token_type=%q expires_in=%d, se esperaba service/300", body.TokenType, body.ExpiresIn)
	}

	ring, kids := keyringFromJWKS(t, s.r)
	p, err := s2stoken.Verify(body.AccessToken, s2stoken.Config{
		Keyring: ring, Audience: "stock-service", Namespace: "mc", RequiredScopes: []string{"stock:read"},
	})
	if err != nil {
		t.Fatalf("el token emitido no valida contra el JWKS publicado: %v", err)
	}
	if p.Subject != s2sIssuer || p.TenantScope != s2stoken.TenantScopeSystem {
		t.Errorf("principal = %+v", p)
	}
	for _, kid := range kids {
		if kid == s.issuerKID {
			t.Fatalf("el kid del emisor %s aparece en el JWKS de iam", kid)
		}
	}
	if err := s.mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestServiceTokenRoute_RechazosConUnicoCodigo(t *testing.T) {
	s := s2sRouter(t)
	other := genKey(t)
	cases := []struct {
		name   string
		form   url.Values
		jti    *int64
		dbErr  error
		status int
	}{
		{"credencial ausente", tokenForm("", "stock-service", "stock:read"), nil, nil, http.StatusUnauthorized},
		{"aserción con la clave de otro servicio", tokenForm(s.assertion(t, other, uuid.NewString()), "stock-service", "stock:read"), nil, nil, http.StatusUnauthorized},
		{"grant_type ajeno", func() url.Values {
			f := tokenForm(s.assertion(t, s.bffKey, uuid.NewString()), "stock-service", "stock:read")
			f.Set("grant_type", "password")
			return f
		}(), nil, nil, http.StatusBadRequest},
		{"client_assertion_type ajeno (invalid_request)", func() url.Values {
			f := tokenForm(s.assertion(t, s.bffKey, uuid.NewString()), "stock-service", "stock:read")
			f.Set("client_assertion_type", "urn:otro")
			return f
		}(), nil, nil, http.StatusBadRequest},
		{"aud fuera del registro", tokenForm(s.assertion(t, s.bffKey, uuid.NewString()), "sales-service", "stock:read"), ptr(1), nil, http.StatusForbidden},
		{"scope fuera del registro", tokenForm(s.assertion(t, s.bffKey, uuid.NewString()), "stock-service", "stock:write"), ptr(1), nil, http.StatusForbidden},
		{"scope amplio prohibido", tokenForm(s.assertion(t, s.bffKey, uuid.NewString()), "stock-service", "system:admin"), ptr(1), nil, http.StatusForbidden},
		{"jti repetido", tokenForm(s.assertion(t, s.bffKey, uuid.NewString()), "stock-service", "stock:read"), ptr(0), nil, http.StatusUnauthorized},
		{"almacén de jti caído", tokenForm(s.assertion(t, s.bffKey, uuid.NewString()), "stock-service", "stock:read"), nil, errors.New("connection refused"), http.StatusServiceUnavailable},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.jti != nil {
				s.expectJTI(*tc.jti, nil)
			}
			if tc.dbErr != nil {
				s.expectJTI(0, tc.dbErr)
			}
			// IP distinta por caso: el cupo pre-auth no interfiere.
			w := s.post(tc.form, "10.1.0."+string(rune('1'+i)))
			if w.Code != tc.status {
				t.Fatalf("→ %d %s, se esperaba %d", w.Code, w.Body.String(), tc.status)
			}
			if strings.Contains(w.Body.String(), "access_token") {
				t.Fatalf("un rechazo devolvió token: %s", w.Body.String())
			}
		})
	}
	if err := s.mock.ExpectationsWereMet(); err != nil {
		t.Errorf("jti escrito de más o de menos: %v", err)
	}
}

func ptr(v int64) *int64 { return &v }

// El cupo pre-auth es por IP y se descuenta antes de verificar: basura repetida desde una IP da
// 429 sin tocar el almacén de jti ni el cupo del emisor que declara.
func TestServiceTokenRoute_RateLimitPreAuthPorIP(t *testing.T) {
	s := s2sRouter(t)
	form := tokenForm("basura", "stock-service", "stock:read")
	var last int
	for i := 0; i < 61; i++ {
		last = s.post(form, "10.2.0.1").Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("request 61 desde la misma IP → %d, se esperaba 429", last)
	}
	if w := s.post(form, "10.2.0.2"); w.Code != http.StatusUnauthorized {
		t.Fatalf("otra IP → %d, se esperaba 401 (el cupo es por IP)", w.Code)
	}
	// X-Forwarded-For no es la IP de origen: rotarlo no renueva el cupo.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/s2s/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	req.RemoteAddr = "10.2.0.1:40000"
	w := httptest.NewRecorder()
	s.r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("X-Forwarded-For rotado evadió el cupo por IP → %d, se esperaba 429", w.Code)
	}
}

// Arranque: una privada en el registro de emisores, o una pública de emisor que comparte kid con
// el keyring de verificación, impiden montar el módulo.
func TestServiceTokenModule_ArranqueRechazaConfigInsegura(t *testing.T) {
	iam := genKey(t)
	bff := genKey(t)

	privRoot := t.TempDir()
	writePEM(t, filepath.Join(privRoot, s2sIssuer), "k.pem", privBlock(t, bff))
	err := accessconfig.SetupServiceTokenModule(gin.New(), accessconfig.ServiceTokenModuleConfig{
		SigningKey: iam, SigningKID: jwtkeyring.KID(&iam.PublicKey), IssuerKeysDir: privRoot,
	})
	if err == nil {
		t.Error("una privada en el registro de emisores no impidió el arranque")
	}

	sharedRoot := t.TempDir()
	writePEM(t, filepath.Join(sharedRoot, s2sIssuer), "k.pem", pubBlock(t, &iam.PublicKey))
	err = accessconfig.SetupServiceTokenModule(gin.New(), accessconfig.ServiceTokenModuleConfig{
		SigningKey: iam, SigningKID: jwtkeyring.KID(&iam.PublicKey), IssuerKeysDir: sharedRoot,
		VerificationKIDs: []string{jwtkeyring.KID(&iam.PublicKey)},
	})
	if err == nil {
		t.Error("una clave de emisor con el kid del keyring de verificación no impidió el arranque")
	}
}

// Arranque: la privada de un emisor puesta en JWT_PUBLIC_KEY_DIR se rechaza al cargar el keyring
// de verificación de iam (en release eso es log.Fatalf en NewAuthModuleConfigFromEnv).
func TestArranque_RechazaPrivadaDeEmisorEnJWTPublicKeyDir(t *testing.T) {
	dir := t.TempDir()
	writePEM(t, dir, "emisor.pem", privBlock(t, genKey(t)))
	t.Setenv("JWT_PUBLIC_KEY_DIR", dir)
	if _, err := identityconfig.LoadVerificationKeyringFromEnv(); err == nil {
		t.Fatal("una privada en JWT_PUBLIC_KEY_DIR no fue rechazada al arranque")
	}
}
