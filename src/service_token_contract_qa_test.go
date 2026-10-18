package main

// ACC-E09 T2 — tests de QA escritos contra el CONTRATO de la épica (no contra la implementación).
// Completan los huecos de service_token_route_test.go:
//   - el token emitido no lleva tenant, usuario ni roles, y declara iss/token_use de servicio;
//   - un token de servicio presentado como bearer de usuario a una ruta protegida de iam → 401;
//   - el cupo por emisor se descuenta DESPUÉS de verificar (basura con iss=<emisor> no lo agota)
//     y existe (el emisor legítimo sí lo agota);
//   - el cupo global pre-auth corta aunque cada IP esté debajo de su cupo;
//   - orden de validación: una aserción mal firmada / vencida / con alg=none / que es un token de
//     usuario de iam se rechaza con 401 sin escribir el jti;
//   - la emisión se audita sin registrar el token ni la aserción;
//   - ningún kid de infra/s2s-keys está en el keyring de verificación (JWT_PUBLIC_KEY_DIR);
//   - s2s-keygen genera un par cuya pública queda en el registro y emite un token de punta a punta.

import (
	"bufio"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/hornosg/go-shared/infrastructure/jwtkeyring"
	"github.com/hornosg/go-shared/infrastructure/s2stoken"

	"github.com/hornosg/iam-service/src/access/infrastructure/servicetoken"
)

// qaAssertion firma una aserción con claims arbitrarios (la base es una aserción válida de
// catalog-bff) para construir los negativos de orden de validación.
func qaAssertion(t *testing.T, key *rsa.PrivateKey, kid string, method jwt.SigningMethod, mutate func(jwt.MapClaims)) string {
	t.Helper()
	now := time.Now()
	c := jwt.MapClaims{
		"iss": s2sIssuer, "sub": s2sIssuer, "aud": s2stoken.AssertionAudience,
		"iat": now.Unix(), "exp": now.Add(30 * time.Second).Unix(), "jti": uuid.NewString(),
	}
	if mutate != nil {
		mutate(c)
	}
	tok := jwt.NewWithClaims(method, c)
	tok.Header["kid"] = kid
	var signed string
	var err error
	if method == jwt.SigningMethodNone {
		signed, err = tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	} else {
		signed, err = tok.SignedString(key)
	}
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

// qaIAMSigningKey recupera la clave de firma de iam que fijó jwksRouter en el entorno.
func qaIAMSigningKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(os.Getenv("JWT_PRIVATE_KEY")))
	if err != nil {
		t.Fatalf("clave de firma de iam: %v", err)
	}
	return k
}

func (s *s2sRoute) issueOK(t *testing.T, ip string) string {
	t.Helper()
	s.expectJTI(1, nil)
	w := s.post(tokenForm(s.assertion(t, s.bffKey, uuid.NewString()), "stock-service", "stock:read"), ip)
	if w.Code != http.StatusOK {
		t.Fatalf("emisión → %d %s, se esperaba 200", w.Code, w.Body.String())
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.AccessToken == "" {
		t.Fatalf("respuesta sin access_token: %s (%v)", w.Body.String(), err)
	}
	return body.AccessToken
}

func TestQA_ServiceToken_ClaimsSinTenantUsuarioNiRoles(t *testing.T) {
	s := s2sRouter(t)
	iamKey := qaIAMSigningKey(t)
	raw := s.issueOK(t, "10.10.0.1")

	claims := jwt.MapClaims{}
	tok, _, err := jwt.NewParser().ParseUnverified(raw, claims)
	if err != nil {
		t.Fatal(err)
	}
	if tok.Method.Alg() != "RS256" {
		t.Errorf("alg = %s, se esperaba RS256", tok.Method.Alg())
	}
	if kid, _ := tok.Header["kid"].(string); kid != jwtkeyring.KID(&iamKey.PublicKey) {
		t.Errorf("kid = %q, se esperaba el kid vigente de iam %q", kid, jwtkeyring.KID(&iamKey.PublicKey))
	}
	want := map[string]string{
		"iss": s2stoken.Issuer, "token_use": s2stoken.TokenUse, "aud": "stock-service", "sub": s2sIssuer,
	}
	for k, v := range want {
		if got, _ := claims[k].(string); got != v {
			t.Errorf("claim %s = %v, se esperaba %q", k, claims[k], v)
		}
	}
	for _, forbidden := range []string{"tenant_id", "tenant", "user_id", "email", "roles", "permissions"} {
		if _, ok := claims[forbidden]; ok {
			t.Errorf("el token de servicio lleva %q (= %v): no debe llevar tenant, usuario ni roles", forbidden, claims[forbidden])
		}
	}
	if fmt.Sprint(claims["scope"]) == "" || strings.Contains(fmt.Sprint(claims["scope"]), "admin") {
		t.Errorf("scope = %v, se esperaba sólo stock:read", claims["scope"])
	}
}

// Un token de servicio firmado con la clave legítima de iam no puede usarse como bearer de usuario.
// Control positivo: un token de usuario con la misma clave NO es rechazado por autenticación.
func TestQA_ServiceToken_ComoBearerDeUsuarioEs401(t *testing.T) {
	s := s2sRouter(t)
	iamKey := qaIAMSigningKey(t)
	svcToken := s.issueOK(t, "10.11.0.1")

	get := func(bearer string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
		req.Header.Set("Authorization", "Bearer "+bearer)
		req.RemoteAddr = "10.11.0.2:40000"
		w := httptest.NewRecorder()
		s.r.ServeHTTP(w, req)
		return w
	}
	if w := get(svcToken); w.Code != http.StatusUnauthorized {
		t.Fatalf("token de servicio como bearer de usuario → %d %s, se esperaba 401", w.Code, w.Body.String())
	}

	userTok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"jti": uuid.NewString(), "iss": s2stoken.UserIssuer, "namespace": "mc",
		"user_id": uuid.NewString(), "email": "qa@example.com", "tenant_id": uuid.NewString(),
		"roles": []string{"system_admin"}, "iat": time.Now().Unix(), "exp": time.Now().Add(5 * time.Minute).Unix(),
	})
	userTok.Header["kid"] = jwtkeyring.KID(&iamKey.PublicKey)
	signed, err := userTok.SignedString(iamKey)
	if err != nil {
		t.Fatal(err)
	}
	if w := get(signed); w.Code == http.StatusUnauthorized {
		t.Fatalf("control: un token de usuario válido también da 401 (%s) — el negativo anterior no prueba nada", w.Body.String())
	}
}

// Orden de validación (forma → firma → exp/iat → jti): ninguna de estas aserciones llega al
// almacén de jti. Sin expectativa en sqlmock, un INSERT inesperado devolvería error → 503, así que
// exigir 401 exacto detecta un jti consumido antes de verificar.
func TestQA_ServiceToken_AsercionesInvalidasNoConsumenJTI(t *testing.T) {
	s := s2sRouter(t)
	iamKey := qaIAMSigningKey(t)
	other := genKey(t)
	now := time.Now()
	cases := map[string]string{
		"firmada con otra clave declarando el kid del emisor": qaAssertion(t, other, s.issuerKID, jwt.SigningMethodRS256, nil),
		"alg=none": qaAssertion(t, nil, s.issuerKID, jwt.SigningMethodNone, nil),
		"vencida con firma válida": qaAssertion(t, s.bffKey, s.issuerKID, jwt.SigningMethodRS256, func(c jwt.MapClaims) {
			c["iat"] = now.Add(-2 * time.Minute).Unix()
			c["exp"] = now.Add(-90 * time.Second).Unix()
		}),
		"vida mayor a 60 s con firma válida": qaAssertion(t, s.bffKey, s.issuerKID, jwt.SigningMethodRS256, func(c jwt.MapClaims) {
			c["exp"] = now.Add(10 * time.Minute).Unix()
		}),
		"iat en el futuro con firma válida": qaAssertion(t, s.bffKey, s.issuerKID, jwt.SigningMethodRS256, func(c jwt.MapClaims) {
			c["iat"] = now.Add(5 * time.Minute).Unix()
			c["exp"] = now.Add(5*time.Minute + 30*time.Second).Unix()
		}),
		"token de usuario de iam como aserción": qaAssertion(t, iamKey, jwtkeyring.KID(&iamKey.PublicKey), jwt.SigningMethodRS256, func(c jwt.MapClaims) {
			c["iss"] = s2stoken.UserIssuer
			c["sub"] = uuid.NewString()
			c["tenant_id"] = uuid.NewString()
		}),
		"iss de otro servicio con la clave del emisor": qaAssertion(t, s.bffKey, s.issuerKID, jwt.SigningMethodRS256, func(c jwt.MapClaims) {
			c["iss"] = "sales-service"
			c["sub"] = "sales-service"
		}),
	}
	i := 0
	for name, a := range cases {
		i++
		ip := fmt.Sprintf("10.12.0.%d", i)
		t.Run(name, func(t *testing.T) {
			w := s.post(tokenForm(a, "stock-service", "stock:read"), ip)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("→ %d %s, se esperaba 401 sin tocar el almacén de jti", w.Code, w.Body.String())
			}
		})
	}
	if err := s.mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// Contrato: «una solicitud que mezcla scopes de usuario y de sistema» → 403. Con la política
// versionada de T2 (catalog-bff sólo tiene scopes de sistema) el scope de usuario además está
// fuera del registro; el caso aislado de mezcla vive en servicetoken/issue_service_token_test.go.
func TestQA_ServiceToken_MezclaDeScopesPorHTTPEs403(t *testing.T) {
	s := s2sRouter(t)
	s.expectJTI(1, nil) // el orden contractual no fija si la política va antes o después del jti
	w := s.post(tokenForm(s.assertion(t, s.bffKey, uuid.NewString()), "stock-service", "stock:read stock:reserve"), "10.13.0.1")
	if w.Code != http.StatusForbidden {
		t.Fatalf("mezcla de scopes → %d %s, se esperaba 403", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "access_token") {
		t.Fatal("un rechazo devolvió token")
	}
}

// El cupo por emisor se descuenta tras verificar: 70 aserciones falsas con iss=catalog-bff desde
// IPs distintas no lo tocan. Si se descontara antes de verificar, el emisor legítimo quedaría en
// 429 (DoS dirigido a un servicio con sólo conocer su nombre).
func TestQA_ServiceToken_CupoPorEmisorNoLoAgotaBasuraConSuIss(t *testing.T) {
	s := s2sRouter(t)
	other := genKey(t)
	for i := 0; i < 70; i++ {
		w := s.post(tokenForm(qaAssertion(t, other, s.issuerKID, jwt.SigningMethodRS256, nil), "stock-service", "stock:read"),
			fmt.Sprintf("10.14.%d.%d", i/200, i%200+1))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("aserción falsa #%d → %d, se esperaba 401", i, w.Code)
		}
	}
	s.issueOK(t, "10.14.9.9")
}

// Y el cupo por emisor existe: el emisor legítimo, rotando IPs (cada una debajo de su cupo), choca
// con su cupo propio antes que con el global.
func TestQA_ServiceToken_CupoPorEmisorTrasVerificar(t *testing.T) {
	s := s2sRouter(t)
	var last *httptest.ResponseRecorder
	limited := false
	for i := 0; i < 200 && !limited; i++ {
		s.expectJTI(1, nil)
		last = s.post(tokenForm(s.assertion(t, s.bffKey, uuid.NewString()), "stock-service", "stock:read"),
			fmt.Sprintf("10.15.%d.%d", i/200, i%200+1))
		switch last.Code {
		case http.StatusOK:
		case http.StatusTooManyRequests:
			limited = true
		default:
			t.Fatalf("emisión legítima #%d → %d %s", i, last.Code, last.Body.String())
		}
	}
	if !limited {
		t.Fatal("200 emisiones legítimas del mismo emisor desde IPs distintas sin 429: no hay cupo por emisor")
	}
	if strings.Contains(last.Body.String(), "access_token") {
		t.Fatal("un 429 devolvió token")
	}
}

// Cupo global pre-auth: cada IP manda una sola request (debajo de su cupo) y aun así el total se
// corta antes de verificar.
func TestQA_ServiceToken_CupoGlobalPreAuth(t *testing.T) {
	s := s2sRouter(t)
	form := tokenForm("basura", "stock-service", "stock:read")
	got429 := false
	for i := 0; i < 2000 && !got429; i++ {
		w := s.post(form, fmt.Sprintf("10.%d.%d.%d", 16+i/60000, (i/250)%250, i%250+1))
		switch w.Code {
		case http.StatusUnauthorized, http.StatusBadRequest:
		case http.StatusTooManyRequests:
			got429 = true
		default:
			t.Fatalf("request #%d → %d", i, w.Code)
		}
	}
	if !got429 {
		t.Fatal("2000 requests desde IPs distintas sin 429: no hay cupo global pre-auth")
	}
}

// Todo token emitido se audita sin registrar el token (ni la aserción). Captura stdout, que es
// donde el composition root escribe el auditor JSON.
func TestQA_ServiceToken_AuditaSinRegistrarElToken(t *testing.T) {
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = wr
	restored := false
	restore := func() {
		if !restored {
			os.Stdout = orig
			wr.Close()
			restored = true
		}
	}
	t.Cleanup(restore)
	out := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(rd)
		out <- string(b)
	}()

	s := s2sRouter(t)
	// Un rechazo no se audita como emisión.
	s.post(tokenForm(qaAssertion(t, genKey(t), s.issuerKID, jwt.SigningMethodRS256, nil), "stock-service", "stock:read"), "10.17.0.1")

	s.expectJTI(1, nil)
	assertion := s.assertion(t, s.bffKey, uuid.NewString())
	w := s.post(tokenForm(assertion, "stock-service", "stock:read"), "10.17.0.2")
	if w.Code != http.StatusOK {
		restore()
		t.Fatalf("emisión → %d %s", w.Code, w.Body.String())
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(body.AccessToken, claims); err != nil {
		t.Fatal(err)
	}
	restore()
	logs := <-out

	events := 0
	sc := bufio.NewScanner(strings.NewReader(logs))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if strings.Contains(sc.Text(), "s2s_token_issued") {
			events++
			if !strings.Contains(sc.Text(), fmt.Sprint(claims["jti"])) {
				t.Errorf("el evento de auditoría no identifica el token por jti: %s", sc.Text())
			}
		}
	}
	if events != 1 {
		t.Errorf("eventos s2s_token_issued = %d, se esperaba 1 (una emisión, un rechazo)", events)
	}
	sig := body.AccessToken[strings.LastIndex(body.AccessToken, ".")+1:]
	asig := assertion[strings.LastIndex(assertion, ".")+1:]
	for what, secret := range map[string]string{"el token": body.AccessToken, "la firma del token": sig, "la aserción": assertion, "la firma de la aserción": asig} {
		if strings.Contains(logs, secret) {
			t.Errorf("la auditoría registró %s", what)
		}
	}
}

// Ningún kid del registro de emisores versionado (infra/s2s-keys) aparece en el keyring de
// verificación que se monta como JWT_PUBLIC_KEY_DIR (infra/jwt-keyring), y ese directorio no
// contiene privadas. Hoy el registro puede estar vacío (catalog-bff entra en T4): el test vigila.
func TestQA_S2SKeys_RegistroDisjuntoDelKeyringVersionado(t *testing.T) {
	lab := filepath.Join("..", "..", "..")
	s2sDir := filepath.Join(lab, "infra", "s2s-keys")
	ringDir := filepath.Join(lab, "infra", "jwt-keyring")
	if _, err := os.Stat(s2sDir); err != nil {
		t.Fatalf("infra/s2s-keys no existe: %v", err)
	}
	kidsIn := func(dir string, recursive bool) map[string]string {
		out := map[string]string{}
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if p != dir && !recursive {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".pem") {
				return nil
			}
			b, _ := os.ReadFile(p)
			blk, _ := pem.Decode(b)
			if blk == nil {
				return nil
			}
			if strings.Contains(blk.Type, "PRIVATE") {
				t.Errorf("clave privada versionada en %s", p)
				return nil
			}
			pub, err := x509.ParsePKIXPublicKey(blk.Bytes)
			if err != nil {
				return nil
			}
			if rk, ok := pub.(*rsa.PublicKey); ok {
				out[jwtkeyring.KID(rk)] = p
			}
			return nil
		})
		return out
	}
	issuers := kidsIn(s2sDir, true)
	ring := kidsIn(ringDir, false)
	if len(issuers) == 0 {
		t.Log("infra/s2s-keys sin claves todavía (la de catalog-bff entra en T4)")
	}
	for kid, p := range issuers {
		if q, ok := ring[kid]; ok {
			t.Errorf("kid %s del registro de emisores (%s) está en el keyring de verificación (%s)", kid, p, q)
		}
	}
	if _, err := servicetoken.LoadIssuerKeys(s2sDir); err != nil {
		t.Errorf("el registro versionado no carga: %v", err)
	}
}

func runKeygen(t *testing.T, env []string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{filepath.Join("..", "scripts", "s2s-keygen.sh")}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	b, err := cmd.CombinedOutput()
	return string(b), err
}

// s2s-keygen genera un par: la pública queda en el registro (<dir>/<servicio>/<kid>.pem) y carga
// como registro de emisores; la privada 0600 fuera del registro; y una aserción firmada con esa
// privada obtiene un token por el router real.
func TestQA_S2SKeygen_GeneraParYEmiteDePuntaAPunta(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl no disponible")
	}
	tmp := t.TempDir()
	pubRoot := filepath.Join(tmp, "s2s-keys")
	privRoot := filepath.Join(tmp, "private")
	out, err := runKeygen(t, []string{"S2S_KEYS_DIR=" + pubRoot, "S2S_PRIVATE_KEY_DIR=" + privRoot}, s2sIssuer)
	if err != nil {
		t.Fatalf("s2s-keygen falló: %v\n%s", err, out)
	}
	pubs, _ := filepath.Glob(filepath.Join(pubRoot, s2sIssuer, "*.pem"))
	privs, _ := filepath.Glob(filepath.Join(privRoot, s2sIssuer, "*.pem"))
	if len(pubs) != 1 || len(privs) != 1 {
		t.Fatalf("se esperaba 1 pública y 1 privada, hay %v / %v\n%s", pubs, privs, out)
	}
	kid := strings.TrimSuffix(filepath.Base(pubs[0]), ".pem")
	if !regexp.MustCompile(`^acc-[0-9a-f]{12}$`).MatchString(kid) {
		t.Errorf("nombre de la pública %q no es un kid acc-<12hex>", kid)
	}
	if st, _ := os.Stat(privs[0]); st.Mode().Perm() != 0o600 {
		t.Errorf("privada con permisos %v, se esperaba 0600", st.Mode().Perm())
	}
	_ = filepath.WalkDir(pubRoot, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if b, _ := os.ReadFile(p); strings.Contains(string(b), "PRIVATE KEY") {
				t.Errorf("material privado en el registro público: %s", p)
			}
		}
		return nil
	})
	reg, err := servicetoken.LoadIssuerKeys(pubRoot)
	if err != nil {
		t.Fatalf("la pública generada no carga como registro de emisores: %v", err)
	}
	if _, ok := reg.PublicKey(s2sIssuer, kid); !ok {
		t.Fatalf("el registro no resuelve (%s, %s)", s2sIssuer, kid)
	}

	// De punta a punta por el router real.
	b, _ := os.ReadFile(privs[0])
	priv, err := jwt.ParseRSAPrivateKeyFromPEM(b)
	if err != nil {
		t.Fatal(err)
	}
	if jwtkeyring.KID(&priv.PublicKey) != kid {
		t.Fatalf("kid del archivo %s ≠ kid de la clave %s", kid, jwtkeyring.KID(&priv.PublicKey))
	}
	jwksRouter(t)
	t.Setenv("S2S_ISSUER_KEYS_DIR", pubRoot)
	appDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { appDB.Close() })
	loginDB, _ := sql.Open("postgres", "postgres://dummy:dummy@127.0.0.1:1/dummy?sslmode=disable")
	t.Cleanup(func() { loginDB.Close() })
	s := &s2sRoute{r: buildRouter(appDB, loginDB), mock: mock, bffKey: priv, issuerKID: kid}
	s.issueOK(t, "10.18.0.1")
}

func TestQA_S2SKeygen_RechazaPrivadaDentroDelRegistroYNombreInvalido(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl no disponible")
	}
	tmp := t.TempDir()
	pubRoot := filepath.Join(tmp, "s2s-keys")
	if out, err := runKeygen(t, []string{"S2S_KEYS_DIR=" + pubRoot, "S2S_PRIVATE_KEY_DIR=" + filepath.Join(pubRoot, "priv")}, "qa-svc"); err == nil {
		t.Fatalf("s2s-keygen aceptó dejar la privada dentro del registro público:\n%s", out)
	}
	_ = filepath.WalkDir(pubRoot, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if b, _ := os.ReadFile(p); strings.Contains(string(b), "PRIVATE KEY") {
				t.Errorf("quedó material privado en el registro: %s", p)
			}
		}
		return nil
	})
	if out, err := runKeygen(t, []string{"S2S_KEYS_DIR=" + pubRoot, "S2S_PRIVATE_KEY_DIR=" + filepath.Join(tmp, "p")}, "../escape"); err == nil {
		t.Fatalf("s2s-keygen aceptó un nombre de servicio con path traversal:\n%s", out)
	}
}
