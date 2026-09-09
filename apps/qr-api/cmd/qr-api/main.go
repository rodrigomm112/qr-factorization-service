// Command qr-api is the composition root: environment, adapters, Fiber app, graceful stop.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/gofiber/fiber/v3"

	httpadapter "github.com/rodrigomm/proyectot/qr-api/internal/adapters/inbound/http"
	"github.com/rodrigomm/proyectot/qr-api/internal/adapters/outbound/statsclient"
	"github.com/rodrigomm/proyectot/qr-api/internal/application"
	"github.com/rodrigomm/proyectot/qr-api/internal/domain/matrix"
	"github.com/rodrigomm/proyectot/qr-api/internal/platform/config"
	"github.com/rodrigomm/proyectot/qr-api/internal/platform/logging"
	"github.com/rodrigomm/proyectot/qr-api/internal/platform/security"
)

// version is injected at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck(os.Getenv, os.Stderr))
	}
	if err := run(); err != nil {
		// No logger yet when the configuration is invalid; the boot failure goes to stderr.
		fmt.Fprintf(os.Stderr, "qr-api: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	logger := logging.New(os.Stdout, cfg.LogLevel).With(
		slog.String("service", "qr-api"),
		slog.String("version", version),
	)

	tokens := security.NewTokenService(security.TokenServiceConfig{
		Secret:           cfg.JWTSecret,
		Issuer:           cfg.JWTIssuer,
		Audience:         cfg.JWTAudience,
		RequiredAudience: config.ServiceAudience,
		TTL:              cfg.JWTTTL,
		Scope:            config.TokenScope,
	})
	credentials := security.NewClientCredentials(cfg.AuthClientID, cfg.AuthClientSecretSHA256)

	stats := statsclient.New(statsclient.Config{
		BaseURL:   cfg.StatsAPIBaseURL,
		Timeout:   cfg.StatsAPITimeout,
		Retries:   cfg.StatsAPIRetries,
		UserAgent: "qr-api/" + version,
		Logger:    logger,
	})

	app := httpadapter.New(cfg, httpadapter.Deps{
		Decompose: application.NewDecomposeAndAnalyze(stats,
			matrix.Limits{MaxRows: cfg.MaxMatrixRows, MaxCols: cfg.MaxMatrixCols}, cfg.RequestBudget, nil),
		Tokens:     application.NewIssueToken(credentials, tokens, config.TokenScope),
		DemoTokens: demoTokens(cfg, tokens),
		Verifier:   tokens,
		Pinger:     stats,
		Logger:     logger,
		Version:    version,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	addr := net.JoinHostPort("0.0.0.0", strconv.Itoa(cfg.Port))
	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening",
			slog.String("addr", addr),
			slog.String("env", cfg.AppEnv),
			slog.String("statsApi", cfg.StatsAPIBaseURL))
		errCh <- app.Listen(addr, fiber.ListenConfig{DisableStartupMessage: true})
	}()

	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("listen: %w", err)
		}
		return nil
	case <-ctx.Done():
		logger.Info("shutdown signal received", slog.Duration("timeout", cfg.ShutdownTimeout))
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := app.ShutdownWithContext(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		logger.Info("stopped cleanly")
		return nil
	}
}

// demoTokens is nil unless DEMO_TOKEN_ENABLED, so the route is absent by default.
func demoTokens(cfg *config.Config, tokens application.TokenIssuer) *application.IssueDemoToken {
	if !cfg.DemoTokenEnabled {
		return nil
	}
	return application.NewIssueDemoToken(tokens, config.TokenScope)
}
