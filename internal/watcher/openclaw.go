package watcher

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/pdasilem/openclaw-multi/internal/shell"
)

type ConfigWriter interface {
	SetCallback(ctx context.Context, route CallbackRoute, url string, localPort int) error
}

type OpenClawWriter struct {
	Executor shell.Executor
	Command  string
	Timeout  time.Duration
}

func (w OpenClawWriter) SetCallback(ctx context.Context, route CallbackRoute, url string, localPort int) error {
	if w.Executor == nil {
		return errors.New("missing OpenClaw executor")
	}
	cmd := w.Command
	if cmd == "" {
		cmd = "openclaw"
	}
	if err := w.run(ctx, []string{cmd, "config", "set", route.URLConfigKey, url}); err != nil {
		return err
	}
	if err := w.run(ctx, []string{cmd, "config", "set", route.PortConfigKey, strconv.Itoa(localPort)}); err != nil {
		return err
	}
	return nil
}

func (w OpenClawWriter) run(ctx context.Context, cmd []string) error {
	_, err := w.Executor.Run(ctx, shell.ExecOpts{Cmd: cmd, Timeout: w.Timeout})
	if err != nil {
		return fmt.Errorf("run %q: %w", cmd, err)
	}
	return nil
}
