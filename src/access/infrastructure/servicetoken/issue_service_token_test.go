package servicetoken

// ACC-E09 T2 — emisión del token de servicio de punta a punta con los adaptadores reales
// (verificador de aserción, registro de emisores, política versionada, firmador RS256) y el
// token emitido verificado por el núcleo s2stoken de go-shared contra el keyring de iam.

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/hornosg/go-shared/infrastructure/jwtkeyring"
	"github.com/hornosg/go-shared/infrastructure/s2stoken"
	"github.com/hornosg/iam-service/src/access/application/usecase"
	"github.com/hornosg/iam-service/src/access/domain/entity"
	"github.com/hornosg/iam-service/src/access/domain/exception"
	"github.com/hornosg/iam-service/src/access/domain/port"
)

var (
	keyOnce               sync.Once
	iamKey, bffKey, other *rsa.PrivateKey
)

func keys(t *testing.T) {
	t.Helper()
	keyOnce.Do(func() {
		for _, k := range []**rsa.PrivateKey{&iamKey, &bffKey, &other} {
			var err error
			if *k, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
				panic(err)
			}
		}
	})
}

func writePub(t *testing.T, dir string, pub *rsa.PublicKey) {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(dir, jwtkeyring.KID(pub)+".pem")
	if err := os.WriteFile(name, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), 0o644); err != nil {
		t.Fatal(err)
	}
}

type memReplay struct {
	seen map[string]bool
	err  error
}

func (m *memReplay) Remember(_ context.Context, jti string, _ time.Time) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	if m.seen[jti] {
		return false, nil
	}
	m.seen[jti] = true
	return true, nil
}

type spyAuditor struct {
	calls    []entity.ServiceTokenClaims
	rejected []port.ServiceTokenRejection
}

func (s *spyAuditor) ServiceTokenRejected(_ context.Context, r port.ServiceTokenRejection) {
	s.rejected = append(s.rejected, r)
}

func (s *spyAuditor) ServiceTokenIssued(_ context.Context, c entity.ServiceTokenClaims, _ string) {
	s.calls = append(s.calls, c)
}

type fixture struct {
	uc      *usecase.IssueServiceTokenUseCase
	replay  *memReplay
	auditor *spyAuditor
	iamRing *jwtkeyring.Keyring
	now     time.Time
}

func newFixture(t *testing.T, policies port.IssuerPolicyRegistry) *fixture {
	t.Helper()
	keys(t)
	root := t.TempDir()
	writePub(t, filepath.Join(root, "catalog-bff-service"), &bffKey.PublicKey)
	reg, err := LoadIssuerKeys(root)
	if err != nil {
		t.Fatal(err)
	}
	iamDir := t.TempDir()
	writePub(t, iamDir, &iamKey.PublicKey)
	ring, err := jwtkeyring.Load(iamDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := AssertDisjoint(reg, ring.KIDs()); err != nil {
		t.Fatal(err)
	}
	if policies == nil {
		if policies, err = NewStaticPolicies(); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().Truncate(time.Second)
	clock := func() time.Time { return now }
	f := &fixture{replay: &memReplay{seen: map[string]bool{}}, auditor: &spyAuditor{}, iamRing: ring, now: now}
	f.uc = usecase.NewIssueServiceTokenUseCase(
		NewAssertionVerifier(reg, clock), f.replay, policies,
		NewRS256Signer(iamKey, jwtkeyring.KID(&iamKey.PublicKey)), f.auditor,
	).WithClock(clock)
	return f
}

type assertionOpts struct {
	iss, sub, aud string
	key           *rsa.PrivateKey
	kid           string
	iat, exp      time.Time
	jti           string
	method        jwt.SigningMethod
}

func (f *fixture) assertion(t *testing.T, mut func(*assertionOpts)) string {
	t.Helper()
	o := assertionOpts{
		iss: "catalog-bff-service", sub: "catalog-bff-service", aud: s2stoken.AssertionAudience,
		key: bffKey, kid: jwtkeyring.KID(&bffKey.PublicKey),
		iat: f.now, exp: f.now.Add(30 * time.Second), jti: uuid.NewString(), method: jwt.SigningMethodRS256,
	}
	if mut != nil {
		mut(&o)
	}
	claims := jwt.MapClaims{"iss": o.iss, "sub": o.sub, "aud": o.aud, "iat": o.iat.Unix(), "exp": o.exp.Unix(), "jti": o.jti}
	tok := jwt.NewWithClaims(o.method, claims)
	tok.Header["kid"] = o.kid
	var signed string
	var err error
	if o.method == jwt.SigningMethodHS256 {
		signed, err = tok.SignedString([]byte("secreto-compartido"))
	} else {
		signed, err = tok.SignedString(o.key)
	}
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func cmd(assertion, aud, scope string) port.IssueServiceTokenCommand {
	return port.IssueServiceTokenCommand{
		GrantType: usecase.GrantTypeClientCredentials, ClientAssertionType: usecase.ClientAssertionTypeJWTBearer,
		ClientAssertion: assertion, Audience: aud, Scope: scope,
	}
}

func TestIssue_EmiteTokenVerificablePorS2Stoken(t *testing.T) {
	f := newFixture(t, nil)
	out, err := f.uc.Issue(context.Background(), cmd(f.assertion(t, nil), "stock-service", "stock:read"))
	if err != nil {
		t.Fatalf("emisión: %v", err)
	}
	p, err := s2stoken.Verify(out.Token, s2stoken.Config{
		Keyring: f.iamRing, Audience: "stock-service", Namespace: "mc",
		RequiredScopes: []string{"stock:read"}, Now: func() time.Time { return f.now },
	})
	if err != nil {
		t.Fatalf("el token emitido no verifica con s2stoken: %v", err)
	}
	if p.Subject != "catalog-bff-service" || p.TenantScope != s2stoken.TenantScopeSystem {
		t.Fatalf("principal inesperado: %+v", p)
	}
	if got := out.Claims.ExpiresAt.Sub(out.Claims.IssuedAt); got != s2stoken.TTL {
		t.Fatalf("TTL %v, se espera %v", got, s2stoken.TTL)
	}
	// Sin tenant, usuario ni roles en el payload.
	var raw jwt.MapClaims
	if _, _, err := jwt.NewParser().ParseUnverified(out.Token, &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"tenant_id", "user_id", "roles", "act"} {
		if _, ok := raw[k]; ok {
			t.Fatalf("el token de servicio no puede llevar %s", k)
		}
	}
	if len(f.auditor.calls) != 1 {
		t.Fatalf("se espera una auditoría por emisión, hubo %d", len(f.auditor.calls))
	}
}

func TestIssue_Rechazos(t *testing.T) {
	f := newFixture(t, nil)
	cases := []struct {
		name string
		mut  func(*assertionOpts)
		aud  string
		sc   string
		want error
	}{
		{"credencial de otro servicio (iss ajeno con la clave de bff)", func(o *assertionOpts) { o.iss, o.sub = "sales-service", "sales-service" }, "stock-service", "stock:read", exception.ErrServiceTokenUnauthenticated},
		{"clave no registrada", func(o *assertionOpts) { o.key, o.kid = other, jwtkeyring.KID(&other.PublicKey) }, "stock-service", "stock:read", exception.ErrServiceTokenUnauthenticated},
		{"kid de bff firmado con otra clave", func(o *assertionOpts) { o.key = other }, "stock-service", "stock:read", exception.ErrServiceTokenUnauthenticated},
		{"HS256", func(o *assertionOpts) { o.method = jwt.SigningMethodHS256 }, "stock-service", "stock:read", exception.ErrServiceTokenUnauthenticated},
		{"iss distinto de sub", func(o *assertionOpts) { o.sub = "otro" }, "stock-service", "stock:read", exception.ErrServiceTokenUnauthenticated},
		{"aud de aserción ajeno", func(o *assertionOpts) { o.aud = "stock-service" }, "stock-service", "stock:read", exception.ErrServiceTokenUnauthenticated},
		{"aserción vencida", func(o *assertionOpts) { o.iat, o.exp = f.now.Add(-2*time.Minute), f.now.Add(-90*time.Second) }, "stock-service", "stock:read", exception.ErrServiceTokenUnauthenticated},
		{"vida mayor a 60s", func(o *assertionOpts) { o.exp = f.now.Add(2 * time.Minute) }, "stock-service", "stock:read", exception.ErrServiceTokenUnauthenticated},
		{"sin jti", func(o *assertionOpts) { o.jti = "" }, "stock-service", "stock:read", exception.ErrServiceTokenUnauthenticated},
		{"aud fuera del registro", nil, "sales-service", "stock:read", exception.ErrServiceTokenForbidden},
		{"scope fuera del registro", nil, "stock-service", "stock:reserve", exception.ErrServiceTokenForbidden},
		{"scope de otro aud", nil, "stock-service", "config:read", exception.ErrServiceTokenForbidden},
		{"scope vacío", nil, "stock-service", "", exception.ErrServiceTokenForbidden},
		{"scope prohibido", nil, "account-service", "system:admin", exception.ErrServiceTokenForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.uc.Issue(context.Background(), cmd(f.assertion(t, tc.mut), tc.aud, tc.sc))
			if !errors.Is(err, tc.want) {
				t.Fatalf("se espera %v, got %v", tc.want, err)
			}
		})
	}
}

func TestIssue_CredencialAusenteYProtocolo(t *testing.T) {
	f := newFixture(t, nil)
	if _, err := f.uc.Issue(context.Background(), cmd("", "stock-service", "stock:read")); !errors.Is(err, exception.ErrServiceTokenUnauthenticated) {
		t.Fatalf("sin aserción: se espera 401, got %v", err)
	}
	c := cmd(f.assertion(t, nil), "stock-service", "stock:read")
	c.GrantType = "password"
	if _, err := f.uc.Issue(context.Background(), c); !errors.Is(err, exception.ErrServiceTokenBadRequest) {
		t.Fatalf("grant_type ajeno: se espera 400, got %v", err)
	}
	// N-3: client_assertion_type ajeno es invalid_request, no unsupported_grant_type.
	c2 := cmd(f.assertion(t, nil), "stock-service", "stock:read")
	c2.ClientAssertionType = "urn:otro"
	if _, err := f.uc.Issue(context.Background(), c2); !errors.Is(err, exception.ErrServiceTokenInvalidRequest) {
		t.Fatalf("client_assertion_type ajeno: se espera invalid_request, got %v", err)
	}
}

func TestIssue_MezclaDeScopesUsuarioYSistemaEs403(t *testing.T) {
	pol, err := newStaticPolicies(map[string][]entity.IssuerPolicy{"mc": {{
		Service: "catalog-bff-service",
		Audiences: map[string]entity.AudiencePolicy{"stock-service": {Scopes: map[string]entity.ScopeClass{
			"stock:read": entity.ScopeClassSystem, "stock:reserve": entity.ScopeClassUser,
		}}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, pol)
	_, err = f.uc.Issue(context.Background(), cmd(f.assertion(t, nil), "stock-service", "stock:read stock:reserve"))
	if !errors.Is(err, exception.ErrServiceTokenForbidden) {
		t.Fatalf("mezcla: se espera 403, got %v", err)
	}
	out, err := f.uc.Issue(context.Background(), cmd(f.assertion(t, nil), "stock-service", "stock:reserve"))
	if err != nil || out.Claims.TenantScope != s2stoken.TenantScopeUser {
		t.Fatalf("scope de usuario: se espera tenant_scope=user, got %+v / %v", out.Claims, err)
	}
}

func TestIssue_JTIRepetidoEs401(t *testing.T) {
	f := newFixture(t, nil)
	a := f.assertion(t, nil)
	if _, err := f.uc.Issue(context.Background(), cmd(a, "stock-service", "stock:read")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.uc.Issue(context.Background(), cmd(a, "stock-service", "stock:read")); !errors.Is(err, exception.ErrServiceTokenUnauthenticated) {
		t.Fatalf("replay: se espera 401, got %v", err)
	}
}

func TestIssue_AlmacenCaidoFallaCerrado(t *testing.T) {
	f := newFixture(t, nil)
	f.replay.err = errors.New("connection refused")
	_, err := f.uc.Issue(context.Background(), cmd(f.assertion(t, nil), "stock-service", "stock:read"))
	if !errors.Is(err, exception.ErrServiceTokenUnavailable) {
		t.Fatalf("almacén caído: se espera 503, got %v", err)
	}
	if len(f.auditor.calls) != 0 {
		t.Fatal("no se emite ni audita con el almacén caído")
	}
}

func TestIssue_AsercionNoVerificadaNoConsumeJTI(t *testing.T) {
	f := newFixture(t, nil)
	jti := uuid.NewString()
	bad := f.assertion(t, func(o *assertionOpts) { o.key = other; o.jti = jti })
	if _, err := f.uc.Issue(context.Background(), cmd(bad, "stock-service", "stock:read")); err == nil {
		t.Fatal("una aserción con firma inválida no puede emitir")
	}
	if f.replay.seen[jti] {
		t.Fatal("el jti de una aserción sin verificar no se escribe (ADR-006)")
	}
}

func TestTokenFirmadoConClaveDeEmisorNoVerificaComoServicio(t *testing.T) {
	f := newFixture(t, nil)
	keys(t)
	claims := &s2stoken.Claims{
		Issuer: s2stoken.Issuer, Subject: "catalog-bff-service", Audience: "stock-service", Scope: "stock:read",
		TokenUse: s2stoken.TokenUse, TenantScope: s2stoken.TenantScopeSystem, Namespace: "mc",
		IssuedAt: jwt.NewNumericDate(f.now), ExpiresAt: jwt.NewNumericDate(f.now.Add(time.Minute)), ID: uuid.NewString(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = jwtkeyring.KID(&bffKey.PublicKey)
	tok.Header["typ"] = s2stoken.TokenType
	forged, err := tok.SignedString(bffKey)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s2stoken.Verify(forged, s2stoken.Config{Keyring: f.iamRing, Audience: "stock-service", Namespace: "mc",
		RequiredScopes: []string{"stock:read"}, Now: func() time.Time { return f.now }})
	if s2stoken.StatusCode(err) != 401 {
		t.Fatalf("token forjado con la clave de un emisor: se espera 401, got %v", err)
	}
}
