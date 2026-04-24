package preflight

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pdasilem/openclaw-multi/internal/shell"
)

// toolsOK returns a MockExecutor that reports all tools as present.
func toolsOK() *shell.MockExecutor {
	return &shell.MockExecutor{
		Responses: []shell.ExecResult{shell.OKResponse("/usr/bin/tool")},
	}
}

func newTestChecker(exec shell.Executor) *Checker {
	c := NewChecker(exec)
	// Point all paths to non-existent files by default — individual tests
	// override as needed via writeFixture.
	c.OSReleasePath = "/nonexistent-os-release"
	c.MeminfoPath = "/nonexistent-meminfo"
	c.ProcNetTCP = "/nonexistent-proc-net-tcp"
	return c
}

// writeFixture writes content to a temp file and returns its path.
func writeFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeFixture: %v", err)
	}
	return path
}

func TestDistroCheckUbuntu22(t *testing.T) {
	c := newTestChecker(toolsOK())
	c.OSReleasePath = writeFixture(t, "ID=ubuntu\nVERSION_ID=\"22.04\"\n")
	r := c.checkDistro()
	if r.Level != LevelOK {
		t.Errorf("Ubuntu 22.04: expected ok, got %s: %s", r.Level, r.Detail)
	}
}

func TestDistroCheckUbuntu20(t *testing.T) {
	c := newTestChecker(toolsOK())
	c.OSReleasePath = writeFixture(t, "ID=ubuntu\nVERSION_ID=\"20.04\"\n")
	r := c.checkDistro()
	if r.Level != LevelWarn {
		t.Errorf("Ubuntu 20.04: expected warn, got %s", r.Level)
	}
}

func TestDistroCheckDebian12(t *testing.T) {
	c := newTestChecker(toolsOK())
	c.OSReleasePath = writeFixture(t, "ID=debian\nVERSION_ID=\"12\"\n")
	r := c.checkDistro()
	if r.Level != LevelOK {
		t.Errorf("Debian 12: expected ok, got %s: %s", r.Level, r.Detail)
	}
}

func TestDistroCheckUnknown(t *testing.T) {
	c := newTestChecker(toolsOK())
	c.OSReleasePath = writeFixture(t, "ID=alpine\nVERSION_ID=\"3.19\"\n")
	r := c.checkDistro()
	if r.Level != LevelWarn {
		t.Errorf("Alpine: expected warn, got %s", r.Level)
	}
}

func TestDiskCheckPass(t *testing.T) {
	c := newTestChecker(toolsOK())
	c.MinDiskGB = 0.000001
	r := c.checkDisk()
	if r.Level == LevelFail {
		t.Errorf("unexpected fail: %s", r.Detail)
	}
}

func TestDiskCheckWarnBelowThreshold(t *testing.T) {
	c := newTestChecker(toolsOK())
	c.MinDiskGB = 1e15
	r := c.checkDisk()
	if r.Level != LevelWarn {
		t.Errorf("expected warn, got %s", r.Level)
	}
}

func TestRAMCheckPass(t *testing.T) {
	c := newTestChecker(toolsOK())
	c.MeminfoPath = writeFixture(t, "MemTotal:       8000000 kB\nMemFree: 4000000 kB\n")
	c.MinRAMGB = 1
	r := c.checkRAM()
	if r.Level != LevelOK {
		t.Errorf("expected ok, got %s: %s", r.Level, r.Detail)
	}
}

func TestRAMCheckWarnBelowThreshold(t *testing.T) {
	c := newTestChecker(toolsOK())
	c.MeminfoPath = writeFixture(t, "MemTotal:       512000 kB\n")
	c.MinRAMGB = 1
	r := c.checkRAM()
	if r.Level != LevelWarn {
		t.Errorf("expected warn for 512 MB, got %s", r.Level)
	}
}

func TestToolsCheckAllPresent(t *testing.T) {
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{shell.OKResponse("/bin/tool")}}
	c := newTestChecker(exec)
	r := c.checkTools(context.Background())
	if r.Level != LevelOK {
		t.Errorf("expected ok, got %s: %s", r.Level, r.Detail)
	}
}

func TestToolsCheckMissing(t *testing.T) {
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{{ExitCode: 1}},
	}
	c := newTestChecker(exec)
	r := c.checkTools(context.Background())
	if r.Level != LevelFail {
		t.Errorf("expected fail, got %s: %s", r.Level, r.Detail)
	}
}

func TestPortConflictDetected(t *testing.T) {
	// /proc/net/tcp format: port 18789 = 0x4965, state 0A = LISTEN
	fixture := "  sl  local_address rem_address st\n" +
		"   0: 0100007F:4965 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0 0 0\n"
	c := newTestChecker(toolsOK())
	c.ProcNetTCP = writeFixture(t, fixture)
	conflicts, err := c.CheckPorts()
	if err != nil {
		t.Fatalf("CheckPorts: %v", err)
	}
	if len(conflicts) != 1 || conflicts[0].Port != 18789 {
		t.Errorf("expected conflict on 18789, got %+v", conflicts)
	}
}

func TestPortNoConflict(t *testing.T) {
	// Port 80 (0x0050) — outside our range.
	fixture := "  sl  local_address rem_address st\n" +
		"   0: 0100007F:0050 00000000:0000 0A 00000000:00000000\n"
	c := newTestChecker(toolsOK())
	c.ProcNetTCP = writeFixture(t, fixture)
	conflicts, err := c.CheckPorts()
	if err != nil {
		t.Fatalf("CheckPorts: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %+v", conflicts)
	}
}

func TestCheckAllReturnsAllResults(t *testing.T) {
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{shell.OKResponse("/bin/tool")}}
	c := newTestChecker(exec)
	c.OSReleasePath = writeFixture(t, "ID=ubuntu\nVERSION_ID=\"22.04\"\n")
	c.MeminfoPath = writeFixture(t, "MemTotal: 4000000 kB\n")
	c.ProcNetTCP = writeFixture(t, "  sl  local_address rem_address st\n")

	results, err := c.CheckAll(context.Background())
	if err != nil {
		t.Fatalf("CheckAll: %v", err)
	}
	if len(results) < 5 {
		t.Errorf("expected ≥5 results, got %d", len(results))
	}
}
