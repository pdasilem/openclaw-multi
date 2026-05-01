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
	CWD     string
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
	Events EventSink
	Actor  string
	Redact []string
}

// Run executes opts.Cmd, capturing stdout and stderr.
// If opts.Sudo is true, prepends "sudo" to the command.
// If opts.Timeout > 0, the command is cancelled after that duration.
func (r *RealExecutor) Run(ctx context.Context, opts ExecOpts) (ExecResult, error) {
	cmd := opts.Cmd
	if len(cmd) == 0 {
		return ExecResult{ExitCode: -1}, fmt.Errorf("command is empty")
	}
	if opts.Sudo {
		cmd = append([]string{"sudo"}, cmd...)
	}
	eventCmd := RedactCommand(cmd, r.Redact)
	started := time.Now().UTC()
	emit(r.Events, Event{Kind: EventStart, Cmd: eventCmd, CWD: opts.CWD, StartedAt: started})

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	c := exec.CommandContext(ctx, cmd[0], cmd[1:]...) //nolint:gosec
	if opts.CWD != "" {
		c.Dir = opts.CWD
	}
	if len(opts.Env) > 0 {
		c.Env = append(c.Environ(), opts.Env...)
	}

	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	err := c.Run()
	exitCode := 0
	if c.ProcessState != nil {
		exitCode = c.ProcessState.ExitCode()
	} else if err != nil {
		exitCode = -1
	}
	res := ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}
	if res.Stdout != "" {
		emit(r.Events, Event{Kind: EventStdout, Cmd: eventCmd, CWD: opts.CWD, Data: res.Stdout})
	}
	if res.Stderr != "" {
		emit(r.Events, Event{Kind: EventStderr, Cmd: eventCmd, CWD: opts.CWD, Data: res.Stderr})
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			r.emitDone(eventCmd, opts.CWD, started, res.ExitCode, "timeout")
			r.emitAudit(cmd, audit.ResultError, "timeout")
			return res, ErrTimeout
		}
		r.emitDone(eventCmd, opts.CWD, started, res.ExitCode, err.Error())
		r.emitAudit(cmd, audit.ResultError, err.Error())
		return res, ErrNonZeroExit{ExitCode: res.ExitCode, Stderr: res.Stderr}
	}

	r.emitDone(eventCmd, opts.CWD, started, res.ExitCode, "")
	r.emitAudit(cmd, audit.ResultOk, "")
	return res, nil
}

func (r *RealExecutor) emitDone(cmd []string, cwd string, started time.Time, exitCode int, errMsg string) {
	emit(r.Events, Event{
		Kind:       EventDone,
		Cmd:        cmd,
		CWD:        cwd,
		ExitCode:   exitCode,
		Error:      errMsg,
		StartedAt:  started,
		DurationMs: time.Since(started).Milliseconds(),
	})
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
		Actor:        r.actor(),
		Action:       audit.ActionShellExec,
		Target:       target,
		Result:       result,
		ErrorMessage: errMsg,
		Details:      map[string]any{"cmd": cmd},
	})
}

func (r *RealExecutor) actor() string {
	if r.Actor != "" {
		return r.Actor
	}
	return "system"
}
