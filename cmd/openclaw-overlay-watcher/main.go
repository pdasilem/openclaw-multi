package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/pdasilem/openclaw-multi/internal/api"
	"github.com/pdasilem/openclaw-multi/internal/shell"
	"github.com/pdasilem/openclaw-multi/internal/watcher"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if _, writeErr := fmt.Fprintln(os.Stderr, err); writeErr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home: %w", err)
	}
	defaultConfig, defaultSnapshot := watcher.DefaultPaths(home)
	fs := flag.NewFlagSet("openclaw-overlay-watcher", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", defaultConfig, "OpenClaw config path")
	snapshotPath := fs.String("snapshot", defaultSnapshot, "watcher snapshot path")
	socketPath := fs.String("socket", api.DefaultSocketPath, "overlay API unix socket path")
	username := fs.String("username", watcher.CurrentUsername(), "managed username")
	debounce := fs.Duration("debounce", 500*time.Millisecond, "config change debounce duration")
	openclawCmd := fs.String("openclaw-bin", "openclaw", "OpenClaw CLI binary")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	source, err := watcher.NewFSNotifySource(filepath.Dir(*configPath))
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()
	svc := watcher.Service{
		Username:     *username,
		ConfigPath:   *configPath,
		SnapshotPath: *snapshotPath,
		FS:           shell.RealFS{},
		Routes:       watcher.NewAPIClient(*socketPath),
		Writer:       watcher.OpenClawWriter{Executor: &shell.RealExecutor{}, Command: *openclawCmd, Timeout: 10 * time.Second},
	}
	return watcher.Loop{
		Service:    svc,
		Source:     source,
		ConfigPath: *configPath,
		Debounce:   *debounce,
		ErrorHandler: func(err error) {
			_, _ = fmt.Fprintln(os.Stderr, err)
		},
	}.Run(ctx)
}
