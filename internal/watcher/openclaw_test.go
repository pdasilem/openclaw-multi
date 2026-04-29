package watcher

import (
	"context"
	"testing"

	"github.com/pdasilem/openclaw-multi/internal/shell"
)

func TestOpenClawWriterSetsURLAndPort(t *testing.T) {
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{shell.OKResponse(""), shell.OKResponse("")}}
	writer := OpenClawWriter{Executor: exec}
	route := CallbackRoute{URLConfigKey: "plugins.entries.calendar.config.callbackUrl", PortConfigKey: "plugins.entries.calendar.config.callbackPort"}
	if err := writer.SetCallback(context.Background(), route, "https://callback.example.com", 19000); err != nil {
		t.Fatalf("SetCallback: %v", err)
	}
	if exec.CallCount() != 2 {
		t.Fatalf("expected 2 calls, got %d", exec.CallCount())
	}
	wantURL := []string{"openclaw", "config", "set", "plugins.entries.calendar.config.callbackUrl", "https://callback.example.com"}
	wantPort := []string{"openclaw", "config", "set", "plugins.entries.calendar.config.callbackPort", "19000"}
	assertCmd(t, exec.Calls[0].Cmd, wantURL)
	assertCmd(t, exec.Calls[1].Cmd, wantPort)
}

func TestOpenClawWriterStopsOnURLFailure(t *testing.T) {
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{{ExitCode: 1}}, Errors: []error{shell.ErrNonZeroExit{ExitCode: 1}}}
	writer := OpenClawWriter{Executor: exec}
	err := writer.SetCallback(context.Background(), CallbackRoute{URLConfigKey: "u", PortConfigKey: "p"}, "https://x", 1)
	if err == nil {
		t.Fatal("expected error")
	}
	if exec.CallCount() != 1 {
		t.Fatalf("expected 1 call, got %d", exec.CallCount())
	}
}

func assertCmd(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("cmd len got %v want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("cmd got %v want %v", got, want)
		}
	}
}
