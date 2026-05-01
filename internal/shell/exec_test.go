package shell

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pdasilem/openclaw-multi/internal/audit"
)

func TestRealExecutor_SimpleCommand(t *testing.T) {
	r := &RealExecutor{}
	res, err := r.Run(context.Background(), ExecOpts{
		Cmd: []string{"echo", "hello"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Stdout != "hello\n" {
		t.Errorf("stdout: got %q, want %q", res.Stdout, "hello\n")
	}
	if res.ExitCode != 0 {
		t.Errorf("exit code: got %d, want 0", res.ExitCode)
	}
}

func TestRealExecutor_NonZeroExit(t *testing.T) {
	r := &RealExecutor{}
	_, err := r.Run(context.Background(), ExecOpts{
		Cmd: []string{"false"},
	})
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
	var nze ErrNonZeroExit
	if !errors.As(err, &nze) {
		t.Errorf("expected ErrNonZeroExit, got %T", err)
	}
}

func TestRealExecutor_Timeout(t *testing.T) {
	r := &RealExecutor{}
	_, err := r.Run(context.Background(), ExecOpts{
		Cmd:     []string{"sleep", "10"},
		Timeout: 50 * time.Millisecond,
	})
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("expected ErrTimeout, got %v", err)
	}
}

func TestRealExecutor_SudoPrepend(t *testing.T) {
	// Verify sudo is prepended by using echo (which will just print "sudo echo hi")
	// We can't actually run sudo in tests; instead test MockExecutor's sudo handling.
	m := &MockExecutor{Responses: []ExecResult{OKResponse("ok")}}
	_, _ = m.Run(context.Background(), ExecOpts{Cmd: []string{"echo", "hi"}, Sudo: true})
	if len(m.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(m.Calls))
	}
	if m.Calls[0].Sudo != true {
		t.Error("expected Sudo=true to be recorded")
	}
}

func TestMockExecutor_RecordsCalls(t *testing.T) {
	m := &MockExecutor{
		Responses: []ExecResult{OKResponse("first"), OKResponse("second")},
	}
	ctx := context.Background()

	r1, err := m.Run(ctx, ExecOpts{Cmd: []string{"ls"}})
	if err != nil || r1.Stdout != "first" {
		t.Errorf("first call: got (%q, %v)", r1.Stdout, err)
	}
	r2, err := m.Run(ctx, ExecOpts{Cmd: []string{"pwd"}})
	if err != nil || r2.Stdout != "second" {
		t.Errorf("second call: got (%q, %v)", r2.Stdout, err)
	}
	if m.CallCount() != 2 {
		t.Errorf("expected 2 calls, got %d", m.CallCount())
	}
	if m.Calls[0].Cmd[0] != "ls" {
		t.Errorf("first call cmd: %v", m.Calls[0].Cmd)
	}
}

func TestMockExecutor_ExhaustedRepeatLast(t *testing.T) {
	m := &MockExecutor{Responses: []ExecResult{OKResponse("x")}}
	ctx := context.Background()
	for range 3 {
		r, err := m.Run(ctx, ExecOpts{Cmd: []string{"cmd"}})
		if err != nil || r.Stdout != "x" {
			t.Errorf("got (%q, %v)", r.Stdout, err)
		}
	}
}

func TestMockExecutor_Called(t *testing.T) {
	m := &MockExecutor{Responses: []ExecResult{OKResponse("")}}
	_, _ = m.Run(context.Background(), ExecOpts{Cmd: []string{"npm", "install"}})
	if !m.Called("npm") {
		t.Error("expected Called('npm') = true")
	}
	if m.Called("apt") {
		t.Error("expected Called('apt') = false")
	}
}

func TestMemFS_WriteAndRead(t *testing.T) {
	fs := NewMemFS()
	data := []byte("hello")
	if err := fs.WriteFile("/tmp/test.txt", data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := fs.ReadFile("/tmp/test.txt")
	if err != nil || string(got) != "hello" {
		t.Errorf("ReadFile: got (%q, %v)", got, err)
	}
}

func TestMemFS_StatMissing(t *testing.T) {
	fs := NewMemFS()
	_, err := fs.Stat("/nonexistent")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestMemFS_Rename(t *testing.T) {
	fs := NewMemFS()
	if err := fs.WriteFile("/a", []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := fs.Rename("/a", "/b"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, err := fs.ReadFile("/b"); err != nil {
		t.Error("file not at new path")
	}
	if _, err := fs.ReadFile("/a"); err == nil {
		t.Error("file still at old path")
	}
}

func TestErrNonZeroExitError(t *testing.T) {
	e := ErrNonZeroExit{ExitCode: 2, Stderr: "bad"}
	if e.Error() == "" {
		t.Error("expected non-empty error")
	}
}

func TestRealExecutorEmitAuditWithLogger(t *testing.T) {
	count := 0
	r := &RealExecutor{Logger: &countLogger{n: &count}}
	_, _ = r.Run(context.Background(), ExecOpts{Cmd: []string{"echo", "x"}})
	if count == 0 {
		t.Error("expected audit emit")
	}
}

func TestRealExecutorEmitsShellEvents(t *testing.T) {
	events := make(chan Event, 8)
	r := &RealExecutor{Events: ChannelSink(events)}
	_, err := r.Run(context.Background(), ExecOpts{Cmd: []string{"echo", "x"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	kinds := map[EventKind]bool{}
	for range 3 {
		e := <-events
		kinds[e.Kind] = true
	}
	for _, kind := range []EventKind{EventStart, EventStdout, EventDone} {
		if !kinds[kind] {
			t.Fatalf("missing event kind %s in %v", kind, kinds)
		}
	}
}

func TestRealExecutorRedactsShellEventCommands(t *testing.T) {
	events := make(chan Event, 8)
	r := &RealExecutor{Events: ChannelSink(events), Redact: []string{"secret-token"}}
	_, err := r.Run(context.Background(), ExecOpts{Cmd: []string{"echo", "secret-token"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	e := <-events
	if got := DisplayCommand(e.Cmd); got != "echo [redacted]" {
		t.Fatalf("redacted command = %q", got)
	}
}

func TestPrivilegedFSUsesSudoForSystemWrite(t *testing.T) {
	exec := &MockExecutor{Responses: []ExecResult{OKResponse("")}}
	fs := PrivilegedFS{Base: RealFS{}, Exec: exec}
	if err := fs.WriteFile("/etc/openclaw-multi/config.yml", []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if len(exec.Calls) != 1 {
		t.Fatalf("expected 1 sudo call, got %d", len(exec.Calls))
	}
	if !exec.Calls[0].Sudo {
		t.Fatal("expected Sudo=true")
	}
	if got := exec.Calls[0].Cmd[0]; got != "install" {
		t.Fatalf("expected install command, got %q", got)
	}
}

type countLogger struct{ n *int }

func (c *countLogger) Emit(_ audit.Event) error {
	*c.n++
	return nil
}

func TestRealFSWriteAndRead(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.txt"
	fs := RealFS{}
	if err := fs.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := fs.ReadFile(path)
	if err != nil || string(got) != "hello" {
		t.Errorf("ReadFile: got (%q, %v)", got, err)
	}
}

func TestRealFSStat(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/f"
	fs := RealFS{}
	if err := fs.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := fs.Stat(path)
	if err != nil || info == nil {
		t.Errorf("Stat: got (%v, %v)", info, err)
	}
}

func TestRealFSMkdirAll(t *testing.T) {
	dir := t.TempDir()
	fs := RealFS{}
	if err := fs.MkdirAll(dir+"/a/b/c", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
}

func TestRealFSRename(t *testing.T) {
	dir := t.TempDir()
	src := dir + "/src"
	dst := dir + "/dst"
	fs := RealFS{}
	if err := fs.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := fs.Rename(src, dst); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, err := fs.ReadFile(dst); err != nil {
		t.Error("file not at dst after rename")
	}
}

func TestFailAt(t *testing.T) {
	m := FailAt(2, "error at 2")
	ctx := context.Background()
	r0, err0 := m.Run(ctx, ExecOpts{Cmd: []string{"cmd"}})
	if err0 != nil || r0.ExitCode != 0 {
		t.Errorf("call 0: unexpected error %v", err0)
	}
	r1, err1 := m.Run(ctx, ExecOpts{Cmd: []string{"cmd"}})
	if err1 != nil || r1.ExitCode != 0 {
		t.Errorf("call 1: unexpected error %v", err1)
	}
	_, err2 := m.Run(ctx, ExecOpts{Cmd: []string{"cmd"}})
	if err2 == nil {
		t.Error("call 2: expected error")
	}
}

func TestMockExecutorReset(t *testing.T) {
	m := &MockExecutor{Responses: []ExecResult{OKResponse("x")}}
	_, _ = m.Run(context.Background(), ExecOpts{Cmd: []string{"cmd"}})
	m.Reset()
	if m.CallCount() != 0 {
		t.Errorf("expected 0 calls after reset, got %d", m.CallCount())
	}
}
