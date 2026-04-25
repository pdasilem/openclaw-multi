package doctor

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pdasilem/openclaw-multi/internal/audit"
	"github.com/pdasilem/openclaw-multi/internal/shell"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

const defaultUmaskProfile = "/etc/profile.d/openclaw-overlay-umask.sh"

// Auditor is the small audit interface used by Checker.
type Auditor interface {
	Emit(e audit.Event) error
}

// Options configures doctor paths.
type Options struct {
	UmaskProfilePath string
}

// Checker coordinates health checks and narrow auto-fixes.
type Checker struct {
	Store  *state.Store
	Exec   shell.Executor
	FS     shell.FS
	Logger Auditor
	Actor  string
	Opts   Options
}

// NewChecker creates a doctor checker.
func NewChecker(store *state.Store, exec shell.Executor, fsys shell.FS, logger Auditor, opts Options) *Checker {
	if opts.UmaskProfilePath == "" {
		opts.UmaskProfilePath = defaultUmaskProfile
	}
	return &Checker{Store: store, Exec: exec, FS: fsys, Logger: logger, Actor: "admin", Opts: opts}
}

// Run executes read-only health checks.
func (c *Checker) Run(ctx context.Context) (Report, error) {
	start := time.Now()
	var report Report
	if err := c.ready(); err != nil {
		return report, err
	}
	users, err := c.Store.ListUsers(ctx)
	if err != nil {
		c.emitRun("health", audit.ResultError, report, err, start)
		return report, err
	}
	c.systemChecks(ctx, &report)
	c.serviceChecks(ctx, &report)
	c.userChecks(ctx, &report, users)
	c.filesystemChecks(&report, users)
	report.Sort()
	c.emitRun("health", audit.ResultOk, report, nil, start)
	return report, nil
}

// RunOpenClawDoctor runs OpenClaw's own doctor for active managed users.
func (c *Checker) RunOpenClawDoctor(ctx context.Context) (Report, error) {
	start := time.Now()
	var report Report
	if err := c.ready(); err != nil {
		return report, err
	}
	users, err := c.Store.ListUsers(ctx)
	if err != nil {
		c.emitRun("openclaw", audit.ResultError, report, err, start)
		return report, err
	}
	for _, user := range users {
		if user.Status == state.UserStatusPaused {
			report.Add(CheckResult{
				ID:       "openclaw-" + user.Username + "-paused",
				Category: "openclaw",
				Target:   user.Username,
				Status:   StatusSkipped,
				Message:  "user is paused",
			})
			continue
		}
		res, err := c.Exec.Run(ctx, shell.ExecOpts{Cmd: []string{"su", "-", user.Username, "-c", "openclaw doctor --json"}})
		if err != nil {
			report.Add(CheckResult{
				ID:       "openclaw-" + user.Username + "-doctor",
				Category: "openclaw",
				Target:   user.Username,
				Status:   StatusFail,
				Message:  "openclaw doctor failed: " + err.Error(),
			})
			continue
		}
		for _, result := range parseDoctorOutput(user.Username, res.Stdout) {
			report.Add(result)
		}
	}
	report.Sort()
	c.emitRun("openclaw", audit.ResultOk, report, nil, start)
	return report, nil
}

// PlanFixes returns approved auto-fixes for fixable report results.
func (c *Checker) PlanFixes(report Report) FixPlan {
	allowed := map[string]bool{
		"chmod-openclaw-dir":    true,
		"chmod-openclaw-config": true,
		"chmod-overlay-dir":     true,
		"repair-umask-profile":  true,
	}
	var plan FixPlan
	for _, result := range report.Results {
		if !result.Fixable || !allowed[result.FixID] {
			continue
		}
		plan.Fixes = append(plan.Fixes, Fix{ID: result.FixID, Target: result.Target, Message: result.Message})
	}
	return plan
}

// ApplyFixes applies only approved permission/profile fixes.
func (c *Checker) ApplyFixes(ctx context.Context, plan FixPlan) error {
	start := time.Now()
	if err := c.ready(); err != nil {
		return err
	}
	for _, fix := range plan.Fixes {
		var err error
		switch fix.ID {
		case "chmod-openclaw-dir":
			_, err = c.Exec.Run(ctx, shell.ExecOpts{Cmd: []string{"chmod", "0700", homePath(fix.Target, ".openclaw")}})
		case "chmod-openclaw-config":
			_, err = c.Exec.Run(ctx, shell.ExecOpts{Cmd: []string{"chmod", "0600", homePath(fix.Target, ".openclaw/openclaw.json")}})
		case "chmod-overlay-dir":
			_, err = c.Exec.Run(ctx, shell.ExecOpts{Cmd: []string{"chmod", "0700", homePath(fix.Target, ".openclaw-overlay")}})
		case "repair-umask-profile":
			err = c.FS.WriteFile(c.Opts.UmaskProfilePath, []byte("umask 0077\n"), 0o644)
		default:
			err = fmt.Errorf("fix %q is not allowed", fix.ID)
		}
		if err != nil {
			c.emitFix(plan, audit.ResultError, err, start)
			return err
		}
	}
	c.emitFix(plan, audit.ResultOk, nil, start)
	return nil
}

func (c *Checker) systemChecks(ctx context.Context, report *Report) {
	if res, err := c.Exec.Run(ctx, shell.ExecOpts{Cmd: []string{"df", "-Pk", "/"}}); err != nil {
		report.Add(result("system-disk", "system", "disk", StatusWarn, "disk check failed: "+err.Error()))
	} else {
		msg, status := parseDF(res.Stdout)
		report.Add(result("system-disk", "system", "disk", status, msg))
	}

	report.Add(fileParseResult(c.FS, "/proc/meminfo", "system-memory", "memory", parseMemInfo))
	report.Add(fileParseResult(c.FS, "/proc/loadavg", "system-load", "load", parseLoad))
	report.Add(fileParseResult(c.FS, "/proc/sys/kernel/yama/ptrace_scope", "system-ptrace", "ptrace", parsePtraceScope))
	report.Add(fileParseResult(c.FS, "/proc/mounts", "system-hidepid", "proc", parseHidepid))

	for _, cmd := range []string{"openclaw", "systemctl", "loginctl", "ss"} {
		if _, err := c.Exec.Run(ctx, shell.ExecOpts{Cmd: []string{"sh", "-c", "command -v " + cmd}}); err != nil {
			report.Add(result("system-command-"+cmd, "system", cmd, StatusWarn, cmd+" not found"))
		} else {
			report.Add(result("system-command-"+cmd, "system", cmd, StatusOK, cmd+" found"))
		}
	}
}

func (c *Checker) serviceChecks(ctx context.Context, report *Report) {
	for _, service := range []string{"tailscaled", "cloudflared", "openclaw-overlay-api"} {
		res, err := c.Exec.Run(ctx, shell.ExecOpts{Cmd: []string{"systemctl", "is-active", service}})
		statusText := strings.TrimSpace(res.Stdout)
		if err == nil && statusText == "active" {
			report.Add(result("service-"+service, "services", service, StatusOK, service+" active"))
			continue
		}
		if service == "openclaw-overlay-api" && (err != nil || statusText == "inactive" || statusText == "unknown" || statusText == "") {
			report.Add(result("service-"+service, "services", service, StatusSkipped, service+" not implemented yet"))
			continue
		}
		report.Add(result("service-"+service, "services", service, StatusFail, service+" not active"))
	}
}

func (c *Checker) userChecks(ctx context.Context, report *Report, users []state.User) {
	for _, user := range users {
		if user.Status == state.UserStatusPaused {
			report.Add(result("user-"+user.Username+"-paused", "users", user.Username, StatusSkipped, "user is paused"))
			continue
		}
		c.checkLinger(ctx, report, user)
		c.checkUserService(ctx, report, user, "openclaw-gateway.service", "gateway")
		c.checkUserService(ctx, report, user, "openclaw-overlay-watcher.service", "watcher")
		c.checkPort(ctx, report, user)
	}
}

func (c *Checker) checkLinger(ctx context.Context, report *Report, user state.User) {
	res, err := c.Exec.Run(ctx, shell.ExecOpts{Cmd: []string{"loginctl", "show-user", user.Username, "-p", "Linger", "--value"}})
	if err != nil {
		report.Add(result("user-"+user.Username+"-linger", "users", user.Username, StatusFail, "linger check failed"))
		return
	}
	if strings.TrimSpace(res.Stdout) == "yes" {
		report.Add(result("user-"+user.Username+"-linger", "users", user.Username, StatusOK, "linger enabled"))
		return
	}
	report.Add(result("user-"+user.Username+"-linger", "users", user.Username, StatusWarn, "linger disabled"))
}

func (c *Checker) checkUserService(ctx context.Context, report *Report, user state.User, service, label string) {
	res, err := c.Exec.Run(ctx, shell.ExecOpts{Cmd: []string{"su", "-", user.Username, "-c", "systemctl --user is-active " + service}})
	if err == nil && strings.TrimSpace(res.Stdout) == "active" {
		report.Add(result("user-"+user.Username+"-"+label, "users", user.Username, StatusOK, label+" active"))
		return
	}
	report.Add(result("user-"+user.Username+"-"+label, "users", user.Username, StatusFail, label+" not active"))
}

func (c *Checker) checkPort(ctx context.Context, report *Report, user state.User) {
	res, err := c.Exec.Run(ctx, shell.ExecOpts{Cmd: []string{"ss", "-ltnp"}})
	if err != nil {
		report.Add(result("user-"+user.Username+"-port", "network", user.Username, StatusWarn, "port check failed"))
		return
	}
	msg, status := parseSSForPort(res.Stdout, user.Port)
	r := result("user-"+user.Username+"-port", "network", user.Username, status, msg)
	r.Details = map[string]string{"port": strconv.Itoa(user.Port)}
	report.Add(r)
}

func (c *Checker) filesystemChecks(report *Report, users []state.User) {
	for _, user := range users {
		c.checkMode(report, user.Username, homePath(user.Username, ".openclaw"), 0o700, "chmod-openclaw-dir")
		c.checkMode(report, user.Username, homePath(user.Username, ".openclaw/openclaw.json"), 0o600, "chmod-openclaw-config")
		c.checkMode(report, user.Username, homePath(user.Username, ".openclaw-overlay"), 0o700, "chmod-overlay-dir")
	}
	data, err := c.FS.ReadFile(c.Opts.UmaskProfilePath)
	if err != nil || strings.TrimSpace(string(data)) != "umask 0077" {
		msg := "overlay umask profile drifted"
		if err != nil {
			msg = "overlay umask profile missing"
		}
		r := result("filesystem-umask-profile", "filesystem", "system", StatusWarn, msg)
		r.Fixable = true
		r.FixID = "repair-umask-profile"
		report.Add(r)
	} else {
		report.Add(result("filesystem-umask-profile", "filesystem", "system", StatusOK, "overlay umask profile ok"))
	}
}

func (c *Checker) checkMode(report *Report, username, path string, expected fs.FileMode, fixID string) {
	info, err := c.FS.Stat(path)
	if err != nil {
		report.Add(result("filesystem-"+username+"-"+filepath.Base(path), "filesystem", username, StatusSkipped, path+" missing"))
		return
	}
	actual := info.Mode().Perm()
	if actual == expected {
		report.Add(result("filesystem-"+username+"-"+filepath.Base(path), "filesystem", username, StatusOK, path+" mode "+modeString(actual)))
		return
	}
	r := result("filesystem-"+username+"-"+filepath.Base(path), "filesystem", username, StatusWarn, path+" mode "+modeString(actual)+", expected "+modeString(expected))
	r.Fixable = true
	r.FixID = fixID
	report.Add(r)
}

func fileParseResult(fsys shell.FS, path, id, target string, parse func(string) (string, Status)) CheckResult {
	data, err := fsys.ReadFile(path)
	if err != nil {
		return result(id, "system", target, StatusWarn, path+" unavailable")
	}
	msg, status := parse(string(data))
	return result(id, "system", target, status, msg)
}

func (c *Checker) ready() error {
	switch {
	case c.Store == nil:
		return errors.New("doctor checker requires state store")
	case c.Exec == nil:
		return errors.New("doctor checker requires executor")
	case c.FS == nil:
		return errors.New("doctor checker requires filesystem")
	}
	return nil
}

func (c *Checker) emitRun(target string, outcome audit.Result, report Report, err error, start time.Time) {
	if c.Logger == nil {
		return
	}
	summary := report.Summary()
	event := audit.Event{
		Actor:      c.Actor,
		Action:     audit.ActionDoctorRun,
		Target:     target,
		Result:     outcome,
		DurationMs: time.Since(start).Milliseconds(),
		Details: map[string]any{
			"ok":      summary.OK,
			"warn":    summary.Warn,
			"fail":    summary.Fail,
			"skipped": summary.Skipped,
		},
	}
	if err != nil {
		event.ErrorMessage = err.Error()
	}
	_ = c.Logger.Emit(event)
}

func (c *Checker) emitFix(plan FixPlan, outcome audit.Result, err error, start time.Time) {
	if c.Logger == nil {
		return
	}
	fixIDs := make([]string, 0, len(plan.Fixes))
	targets := make([]string, 0, len(plan.Fixes))
	for _, fix := range plan.Fixes {
		fixIDs = append(fixIDs, fix.ID)
		targets = append(targets, fix.Target)
	}
	event := audit.Event{
		Actor:      c.Actor,
		Action:     audit.ActionDoctorFix,
		Target:     "health",
		Result:     outcome,
		DurationMs: time.Since(start).Milliseconds(),
		Details:    map[string]any{"fix_ids": fixIDs, "targets": targets},
	}
	if err != nil {
		event.ErrorMessage = err.Error()
	}
	_ = c.Logger.Emit(event)
}

func result(id, category, target string, status Status, message string) CheckResult {
	return CheckResult{ID: id, Category: category, Target: target, Status: status, Message: message}
}

func homePath(username, suffix string) string {
	return filepath.Join("/home", username, suffix)
}

func modeString(mode fs.FileMode) string {
	return fmt.Sprintf("%04o", mode.Perm())
}
