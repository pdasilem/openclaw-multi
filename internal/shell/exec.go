// Package shell provides the Executor interface — the single abstraction for
// all subprocess calls in openclaw-multi. Real code uses RealExecutor; tests
// use MockExecutor. Nothing in this project calls os/exec directly.
package shell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/pdasilem/openclaw-multi/internal/audit"
)

// Logger is the minimal interface shell.RealExecutor needs for audit logging.
// *audit.Logger satisfies this interface.
type Logger interface {
	Emit(e audit.Event) error
}

// ErrTimeout is returned when a command exceeds its deadline.
var ErrTimeout = errors.New("command timed out")

// ErrNonZeroExit is returned when a command exits with a non-zero code.
type ErrNonZeroExit struct {
	ExitCode int
	Stderr   string
}

func (e ErrNonZeroExit) Error() string {
	return fmt.Sprintf("exit %d: %s", e.ExitCode, e.Stderr)
}

// ExecOpts configures a single subprocess invocation.
type ExecOpts struct {
	Cmd     []string
	Timeout time.Duration // 0 = no timeout
	Sudo    bool
	Env     []string // additional env vars (added to os environment)
}

// ExecResult holds the captured output of a completed command.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Executor is the interface every package uses to run subprocesses.
type Executor interface {
	Run(ctx context.Context, opts ExecOpts) (ExecResult, error)
}

// RealExecutor runs commands via os/exec and emits audit log entries.
type RealExecutor struct {
	Logger Logger
}

// Run executes opts.Cmd, capturing stdout and stderr.
// If opts.Sudo is true, prepends "sudo" to the command.
// If opts.Timeout > 0, the command is cancelled after that duration.
func (r *RealExecutor) Run(ctx context.Context, opts ExecOpts) (ExecResult, error) {
	cmd := opts.Cmd
	if opts.Sudo {
		cmd = append([]string{"sudo"}, cmd...)
	}

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	c := exec.CommandContext(ctx, cmd[0], cmd[1:]...) //nolint:gosec
	if len(opts.Env) > 0 {
		c.Env = append(c.Environ(), opts.Env...)
	}

	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	err := c.Run()
	res := ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: c.ProcessState.ExitCode(),
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			r.emitAudit(opts.Cmd, audit.ResultError, "timeout")
			return res, ErrTimeout
		}
		r.emitAudit(opts.Cmd, audit.ResultError, err.Error())
		return res, ErrNonZeroExit{ExitCode: res.ExitCode, Stderr: res.Stderr}
	}

	r.emitAudit(opts.Cmd, audit.ResultOk, "")
	return res, nil
}

func (r *RealExecutor) emitAudit(cmd []string, result audit.Result, errMsg string) {
	if r.Logger == nil {
		return
	}
	target := ""
	if len(cmd) > 0 {
		target = cmd[0]
	}
	_ = r.Logger.Emit(audit.Event{
		Actor:        "system",
		Action:       audit.ActionShellExec,
		Target:       target,
		Result:       result,
		ErrorMessage: errMsg,
		Details:      map[string]any{"cmd": cmd},
	})
}
