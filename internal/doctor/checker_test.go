package doctor

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/pdasilem/openclaw-multi/internal/audit"
	"github.com/pdasilem/openclaw-multi/internal/shell"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

type auditRecorder struct {
	events []audit.Event
}

func (r *auditRecorder) Emit(e audit.Event) error {
	r.events = append(r.events, e)
	return nil
}

func TestReportSummaryAndSort(t *testing.T) {
	report := Report{Results: []CheckResult{
		{ID: "b", Category: "openclaw", Target: "bob", Status: StatusSkipped},
		{ID: "a", Category: "system", Target: "disk", Status: StatusOK},
		{ID: "c", Category: "filesystem", Target: "alice", Status: StatusWarn},
		{ID: "d", Category: "users", Target: "alice", Status: StatusFail},
	}}
	report.Sort()
	if report.Results[0].Category != "system" {
		t.Fatalf("unexpected order: %+v", report.Results)
	}
	summary := report.Summary()
	if summary.OK != 1 || summary.Warn != 1 || summary.Fail != 1 || summary.Skipped != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestRunReportsHealthAndFixablePermissions(t *testing.T) {
	ctx := context.Background()
	store := openDoctorTestStore(t)
	if err := store.UpsertUser(ctx, state.User{Username: "alice", Port: 18789, Status: state.UserStatusActive}); err != nil {
		t.Fatalf("UpsertUser alice: %v", err)
	}
	if err := store.UpsertUser(ctx, state.User{Username: "bob", Port: 18790, Status: state.UserStatusPaused}); err != nil {
		t.Fatalf("UpsertUser bob: %v", err)
	}
	mem := healthyFS()
	mem.Files["/home/alice/.openclaw"] = nil
	mem.Modes["/home/alice/.openclaw"] = fs.ModeDir | 0o755
	mem.Files["/home/alice/.openclaw/openclaw.json"] = []byte("{}")
	mem.Modes["/home/alice/.openclaw/openclaw.json"] = 0o644
	mem.Files["/home/alice/.openclaw-overlay"] = nil
	mem.Modes["/home/alice/.openclaw-overlay"] = fs.ModeDir | 0o700
	log := &auditRecorder{}
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{
		shell.OKResponse("Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/root 100000 1000 99000 1% /\n"),
		shell.OKResponse("/usr/bin/systemctl\n"),
		shell.OKResponse("/usr/bin/loginctl\n"),
		shell.OKResponse("/usr/bin/ss\n"),
		shell.OKResponse("active\n"),
		shell.OKResponse("active\n"),
		shell.OKResponse("inactive\n"),
		shell.OKResponse("yes\n"),
		shell.OKResponse("2026.4.27\n"),
		shell.OKResponse("active\n"),
		shell.OKResponse("active\n"),
		shell.OKResponse("LISTEN 0 4096 127.0.0.1:18789 0.0.0.0:*\n"),
	}}
	checker := NewChecker(store, exec, mem, log, Options{})
	report, err := checker.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Summary().Warn == 0 {
		t.Fatalf("expected warning for unsafe permissions: %+v", report.Results)
	}
	if !hasFix(report, "chmod-openclaw-config") || !hasFix(report, "chmod-openclaw-dir") {
		t.Fatalf("expected chmod fixes: %+v", report.Results)
	}
	if !hasStatus(report, "users", "bob", StatusSkipped) {
		t.Fatalf("expected paused bob skipped: %+v", report.Results)
	}
	if len(log.events) == 0 || log.events[0].Action != audit.ActionDoctorRun {
		t.Fatalf("expected doctor_run audit event: %+v", log.events)
	}
}

func TestRunOpenClawDoctorParsesJSONAndSkipsPaused(t *testing.T) {
	ctx := context.Background()
	store := openDoctorTestStore(t)
	if err := store.UpsertUser(ctx, state.User{Username: "alice", Status: state.UserStatusActive}); err != nil {
		t.Fatalf("UpsertUser alice: %v", err)
	}
	if err := store.UpsertUser(ctx, state.User{Username: "bob", Status: state.UserStatusPaused}); err != nil {
		t.Fatalf("UpsertUser bob: %v", err)
	}
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{
		shell.OKResponse(`{"checks":[{"id":"config","status":"ok","message":"config valid"},{"id":"plugins","status":"warn","message":"plugin warning"}]}`),
	}}
	report, err := NewChecker(store, exec, shell.NewMemFS(), nil, Options{}).RunOpenClawDoctor(ctx)
	if err != nil {
		t.Fatalf("RunOpenClawDoctor: %v", err)
	}
	if report.Summary().OK != 1 || report.Summary().Warn != 1 || report.Summary().Skipped != 1 {
		t.Fatalf("unexpected summary: %+v results=%+v", report.Summary(), report.Results)
	}
}

func TestRunOpenClawDoctorFallsBackToTextAndHandlesFailure(t *testing.T) {
	ctx := context.Background()
	store := openDoctorTestStore(t)
	if err := store.UpsertUser(ctx, state.User{Username: "alice", Status: state.UserStatusActive}); err != nil {
		t.Fatalf("UpsertUser alice: %v", err)
	}
	report, err := NewChecker(store, shell.FailAt(0, "doctor failed"), shell.NewMemFS(), nil, Options{}).RunOpenClawDoctor(ctx)
	if err != nil {
		t.Fatalf("RunOpenClawDoctor: %v", err)
	}
	if report.Summary().Fail != 1 {
		t.Fatalf("expected doctor failure result: %+v", report.Results)
	}

	exec := &shell.MockExecutor{Responses: []shell.ExecResult{shell.OKResponse("WARN plugin missing\n")}}
	report, err = NewChecker(store, exec, shell.NewMemFS(), nil, Options{}).RunOpenClawDoctor(ctx)
	if err != nil {
		t.Fatalf("RunOpenClawDoctor text: %v", err)
	}
	if report.Summary().Warn != 1 {
		t.Fatalf("expected text warning: %+v", report.Results)
	}
}

func TestPlanAndApplyFixes(t *testing.T) {
	report := Report{Results: []CheckResult{
		{Target: "alice", Message: "config mode wrong", Fixable: true, FixID: "chmod-openclaw-config"},
		{Target: "alice", Message: "not allowed", Fixable: true, FixID: "rotate-key"},
		{Target: "system", Message: "profile missing", Fixable: true, FixID: "repair-umask-profile"},
	}}
	memFS := shell.NewMemFS()
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{shell.OKResponse("")}}
	checker := NewChecker(openDoctorTestStore(t), exec, memFS, nil, Options{})
	plan := checker.PlanFixes(report)
	if len(plan.Fixes) != 2 {
		t.Fatalf("expected two allowed fixes, got %+v", plan)
	}
	if err := checker.ApplyFixes(context.Background(), plan); err != nil {
		t.Fatalf("ApplyFixes: %v", err)
	}
	if exec.CallCount() != 1 {
		t.Fatalf("expected one chmod command, got %d", exec.CallCount())
	}
	if string(memFS.Files[defaultUmaskProfile]) != "umask 0077\n" {
		t.Fatalf("expected umask profile repair")
	}
}

func TestApplyFixesRejectsUnknownFix(t *testing.T) {
	err := NewChecker(openDoctorTestStore(t), &shell.MockExecutor{}, shell.NewMemFS(), nil, Options{}).
		ApplyFixes(context.Background(), FixPlan{Fixes: []Fix{{ID: "rotate-key", Target: "system"}}})
	if err == nil {
		t.Fatal("expected unknown fix error")
	}
}

func TestCheckerReadyRejectsMissingDependencies(t *testing.T) {
	if _, err := (&Checker{}).Run(context.Background()); err == nil {
		t.Fatal("expected missing store error")
	}
	if _, err := (&Checker{Store: openDoctorTestStore(t)}).Run(context.Background()); err == nil {
		t.Fatal("expected missing executor error")
	}
	if _, err := (&Checker{Store: openDoctorTestStore(t), Exec: &shell.MockExecutor{}}).Run(context.Background()); err == nil {
		t.Fatal("expected missing filesystem error")
	}
}

func TestParsers(t *testing.T) {
	if _, status := parseDF("bad"); status != StatusWarn {
		t.Fatal("expected malformed df warning")
	}
	if _, status := parseSSForPort("LISTEN 0 0 0.0.0.0:18789 0.0.0.0:*", 18789); status != StatusWarn {
		t.Fatal("expected public listener warning")
	}
	if _, status := parseSSForPort("", 18789); status != StatusFail {
		t.Fatal("expected missing listener failure")
	}
	if normalizeStatus("success") != StatusOK || normalizeStatus("error") != StatusFail {
		t.Fatal("unexpected status normalization")
	}
}

func openDoctorTestStore(t *testing.T) *state.Store {
	t.Helper()
	store, err := state.Open(context.Background(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("state.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func healthyFS() *shell.MemFS {
	fsys := shell.NewMemFS()
	fsys.Files["/proc/meminfo"] = []byte("MemTotal: 4096000 kB\nMemAvailable: 2048000 kB\n")
	fsys.Files["/proc/loadavg"] = []byte("0.10 0.20 0.30 1/100 123\n")
	fsys.Files["/proc/sys/kernel/yama/ptrace_scope"] = []byte("2\n")
	fsys.Files["/proc/mounts"] = []byte("proc /proc proc rw,nosuid,nodev,noexec,relatime,hidepid=2 0 0\n")
	fsys.Files[defaultUmaskProfile] = []byte("umask 0077\n")
	return fsys
}

func hasFix(report Report, fixID string) bool {
	for _, result := range report.Results {
		if result.FixID == fixID {
			return true
		}
	}
	return false
}

func hasStatus(report Report, category, target string, status Status) bool {
	for _, result := range report.Results {
		if result.Category == category && result.Target == target && result.Status == status {
			return true
		}
	}
	return false
}

type statOnlyFS struct {
	*shell.MemFS
	err error
}

func (s statOnlyFS) Stat(string) (fs.FileInfo, error) {
	return nil, s.err
}

func TestFileParseResultWarnsOnMissingFile(t *testing.T) {
	r := fileParseResult(statOnlyFS{MemFS: shell.NewMemFS(), err: errors.New("missing")}, "/missing", "id", "target", parseLoad)
	if r.Status != StatusWarn {
		t.Fatalf("expected warning, got %+v", r)
	}
}
