package config

import (
	"context"
	"crypto/rsa"
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/hornosg/iam-service/src/access/application/usecase"
	"github.com/hornosg/iam-service/src/access/infrastructure/controller"
	"github.com/hornosg/iam-service/src/access/infrastructure/servicetoken"
)

// Cupos del endpoint de emisión (ADR-006 «Rate limit pre-auth»). Un emisor cachea su token por
// (aud, scope) y renueva antes de los 300 s: 60 por minuto deja margen de sobra a la flota.
const (
	serviceTokenWindow       = time.Minute
	serviceTokenPerIP        = 60
	serviceTokenGlobal       = 600
	serviceTokenPerIssuer    = 60
	serviceTokenPurgeEvery   = 5 * time.Minute
	serviceTokenPurgeTimeout = 10 * time.Second
)

// ServiceTokenModuleConfig es lo que el composition root le pasa al módulo. La clave de firma
// llega cruda (*rsa.PrivateKey + kid): access no importa identity.
type ServiceTokenModuleConfig struct {
	DB               *sql.DB
	SigningKey       *rsa.PrivateKey
	SigningKID       string
	VerificationKIDs []string // keyring de verificación de tokens de usuario + kid de firma
	IssuerKeysDir    string   // S2S_ISSUER_KEYS_DIR: <dir>/<servicio>/*.pem, sólo públicas
	Now              func() time.Time
	PurgeEvery       time.Duration // 0 = sin job de purga (tests)
}

// SetupServiceTokenModule monta POST /api/v1/s2s/token. Un error es configuración insegura
// (privada en el registro de emisores, kid compartido con el keyring de verificación, política
// inválida) y el servicio no debe arrancar. Sin clave de firma la ruta existe y responde 503.
func SetupServiceTokenModule(r gin.IRoutes, cfg ServiceTokenModuleConfig) error {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	issuerKeys, err := servicetoken.LoadIssuerKeys(cfg.IssuerKeysDir)
	if err != nil {
		return fmt.Errorf("s2s token: registro de emisores: %w", err)
	}
	if err := servicetoken.AssertDisjoint(issuerKeys, cfg.VerificationKIDs); err != nil {
		return fmt.Errorf("s2s token: %w", err)
	}
	policies, err := servicetoken.NewStaticPolicies()
	if err != nil {
		return fmt.Errorf("s2s token: política: %w", err)
	}
	perIP := servicetoken.NewWindowLimiter(serviceTokenPerIP, serviceTokenWindow, cfg.Now)
	global := servicetoken.NewWindowLimiter(serviceTokenGlobal, serviceTokenWindow, cfg.Now)

	if cfg.SigningKey == nil {
		log.Printf("SECURITY WARNING: s2s token sin clave de firma — %s responde 503 (fail-closed)", controller.ServiceTokenPath)
		r.POST(controller.ServiceTokenPath, controller.NewServiceTokenHandler(nil, perIP, global).Issue)
		return nil
	}

	replay := servicetoken.NewPostgresReplayStore(cfg.DB)
	uc := usecase.NewIssueServiceTokenUseCase(
		servicetoken.NewAssertionVerifier(issuerKeys, cfg.Now), replay, policies,
		servicetoken.NewRS256Signer(cfg.SigningKey, cfg.SigningKID),
		servicetoken.NewSlogAuditor(slog.New(slog.NewJSONHandler(os.Stdout, nil))),
	).WithIssuerLimiter(servicetoken.NewWindowLimiter(serviceTokenPerIssuer, serviceTokenWindow, cfg.Now)).WithClock(cfg.Now)
	r.POST(controller.ServiceTokenPath, controller.NewServiceTokenHandler(uc, perIP, global).Issue)

	if cfg.PurgeEvery > 0 {
		go purgeReplayStore(replay, cfg.PurgeEvery, cfg.Now)
	}
	return nil
}

func purgeReplayStore(store *servicetoken.PostgresReplayStore, every time.Duration, now func() time.Time) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), serviceTokenPurgeTimeout)
		if n, err := store.Purge(ctx, now()); err != nil {
			log.Printf("s2s token: purga de jti falló: %v", err)
		} else if n > 0 {
			log.Printf("s2s token: purga de jti: %d filas", n)
		}
		cancel()
	}
}
