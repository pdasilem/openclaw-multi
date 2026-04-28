package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pdasilem/openclaw-multi/internal/api"
	"github.com/pdasilem/openclaw-multi/internal/audit"
	"github.com/pdasilem/openclaw-multi/internal/cloudflared"
	"github.com/pdasilem/openclaw-multi/internal/config"
	"github.com/pdasilem/openclaw-multi/internal/shell"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

func main() {
	if err := run(); err != nil {
		if _, writeErr := fmt.Fprintln(os.Stderr, err); writeErr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "/etc/openclaw-multi/config.yml", "overlay config path")
	statePath := flag.String("state", "/var/lib/openclaw-multi/state.db", "state database path")
	socketPath := flag.String("socket", api.DefaultSocketPath, "unix socket path")
	cloudflaredPath := flag.String("cloudflared-config", cloudflared.DefaultConfigPath, "cloudflared config path")
	auditPath := flag.String("audit-log", "/var/log/openclaw-multi/audit.log", "audit log path")
	validateCommand := flag.String("validate-command", "", "override validate command")
	sighupCommand := flag.String("sighup-command", "", "override SIGHUP command")
	skipCredentialCheck := flag.Bool("skip-credential-check", false, "skip cloudflared credentials file existence check")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	cfg, err := config.Load(*configPath)
	if err != nil && !errors.Is(err, config.ErrNotFound) {
		return err
	}
	store, err := state.Open(ctx, *statePath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	logger, err := audit.New(*auditPath)
	if err != nil {
		return err
	}
	defer func() { _ = logger.Close() }()
	_ = logger.Emit(audit.Event{Actor: "system", Action: audit.ActionStartup, Target: "openclaw-overlay-api", Result: audit.ResultOk})
	defer func() {
		_ = logger.Emit(audit.Event{Actor: "system", Action: audit.ActionShutdown, Target: "openclaw-overlay-api", Result: audit.ResultOk})
	}()

	manager := &cloudflared.Manager{
		Config:              cfg,
		ConfigPath:          *cloudflaredPath,
		FS:                  shell.RealFS{},
		Executor:            &shell.RealExecutor{Logger: logger},
		Auditor:             logger,
		Actor:               "system",
		ValidateCmd:         splitCommand(*validateCommand),
		SIGHUPCmd:           splitCommand(*sighupCommand),
		SkipCredentialCheck: *skipCredentialCheck,
	}
	routeService := api.RouteService{Store: store, Config: cfg, Publisher: manager}
	server := api.Server{Store: store, Routes: routeService, Peers: api.UnixPeerCredentialsProvider{}, Auditor: logger}
	ln, err := api.ListenUnix(*socketPath)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(*socketPath) }()
	httpServer := &http.Server{Handler: server.Handler(), ConnContext: api.ContextWithPeer, ReadHeaderTimeout: 5 * time.Second}
	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.Serve(ln) }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func splitCommand(raw string) []string {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return nil
	}
	return fields
}
