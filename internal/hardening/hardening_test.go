package hardening

import (
	"context"
	"testing"

	"github.com/pdasilem/openclaw-multi/internal/audit"
	"github.com/pdasilem/openclaw-multi/internal/shell"
)

func newTestHardener(exec shell.Executor, fs shell.FS) *Hardener {
	return &Hardener{Exec: exec, FS: fs}
}

func okExec() *shell.MockExecutor {
	return &shell.MockExecutor{Responses: []shell.ExecResult{shell.OKResponse("")}}
}

func TestApplySysctlWritesFile(t *testing.T) {
	fs := shell.NewMemFS()
	h := newTestHardener(okExec(), fs)
	if err := h.ApplySysctl(context.Background()); err != nil {
		t.Fatalf("ApplySysctl: %v", err)
	}
	data, err := fs.ReadFile(sysctlConf)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != string(sysctlContent) {
		t.Errorf("content mismatch")
	}
}

func TestApplySysctlIdempotent(t *testing.T) {
	fs := shell.NewMemFS()
	exec := okExec()
	h := newTestHardener(exec, fs)

	// First call.
	if err := h.ApplySysctl(context.Background()); err != nil {
		t.Fatalf("first ApplySysctl: %v", err)
	}
	callsAfterFirst := exec.CallCount()

	// Second call — file already correct, should not call sysctl --system again.
	if err := h.ApplySysctl(context.Background()); err != nil {
		t.Fatalf("second ApplySysctl: %v", err)
	}
	if exec.CallCount() != callsAfterFirst {
		t.Errorf("idempotent: executor called again on second run (%d vs %d)",
			callsAfterFirst, exec.CallCount())
	}
}

func TestApplyHidepidAppendsLine(t *testing.T) {
	fs := shell.NewMemFS()
	if err := fs.WriteFile(fstabPath, []byte("# existing fstab\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := newTestHardener(okExec(), fs)
	if err := h.ApplyHidepid(context.Background()); err != nil {
		t.Fatalf("ApplyHidepid: %v", err)
	}
	data, _ := fs.ReadFile(fstabPath)
	if !containsStr(string(data), hidepidLine) {
		t.Errorf("hidepid line not found in fstab: %s", string(data))
	}
}

func TestApplyHidepidIdempotent(t *testing.T) {
	fs := shell.NewMemFS()
	// Pre-populate fstab with the hidepid line already present.
	if err := fs.WriteFile(fstabPath, []byte(hidepidLine+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exec := okExec()
	h := newTestHardener(exec, fs)
	if err := h.ApplyHidepid(context.Background()); err != nil {
		t.Fatalf("ApplyHidepid: %v", err)
	}
	// Should not have called mount remount.
	if exec.Called("mount") {
		t.Error("expected no mount call when hidepid already present")
	}
}

func TestApplyProfileCreatesFile(t *testing.T) {
	fs := shell.NewMemFS()
	h := newTestHardener(okExec(), fs)
	if err := h.ApplyProfile(context.Background()); err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}
	data, err := fs.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("ReadFile profile: %v", err)
	}
	if string(data) != string(profileContent) {
		t.Errorf("profile content mismatch")
	}
}

func TestApplyProfileIdempotentContent(t *testing.T) {
	fs := shell.NewMemFS()
	if err := fs.WriteFile(profilePath, profileContent, 0o644); err != nil {
		t.Fatal(err)
	}
	exec := okExec()
	h := newTestHardener(exec, fs)
	// Second call — profile already correct, write should be skipped,
	// but mkdir+chmod are always called.
	if err := h.ApplyProfile(context.Background()); err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}
	// mkdir and chmod should still be called.
	if !exec.Called("mkdir") {
		t.Error("expected mkdir to be called")
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && findSub(s, sub)
}

func findSub(s, sub string) bool {
	for i := range len(s) - len(sub) + 1 {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestApplyProfileEmitWithLogger(t *testing.T) {
	count := 0
	logger := &countAuditLogger{n: &count}
	fs := shell.NewMemFS()
	if err := fs.WriteFile("/etc/fstab", []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &Hardener{Exec: okExec(), FS: fs, Logger: logger}
	_ = h.ApplyProfile(context.Background())
	if count == 0 {
		t.Error("expected audit emit")
	}
}

type countAuditLogger struct{ n *int }

func (c *countAuditLogger) Emit(_ audit.Event) error {
	*c.n++
	return nil
}
