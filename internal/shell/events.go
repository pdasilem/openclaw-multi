package shell

import (
	"strings"
	"time"
)

// EventKind classifies command lifecycle output for the embedded terminal.
type EventKind string

const (
	EventStart  EventKind = "start"
	EventStdout EventKind = "stdout"
	EventStderr EventKind = "stderr"
	EventDone   EventKind = "done"
)

// Event is one command terminal event.
type Event struct {
	Kind       EventKind
	Cmd        []string
	CWD        string
	Data       string
	ExitCode   int
	Error      string
	StartedAt  time.Time
	DurationMs int64
}

// EventSink receives command terminal events.
type EventSink interface {
	EmitShellEvent(Event)
}

// ChannelSink adapts a channel to EventSink.
type ChannelSink chan<- Event

// EmitShellEvent sends e without blocking the command path.
func (c ChannelSink) EmitShellEvent(e Event) {
	select {
	case c <- e:
	default:
	}
}

// DisplayCommand renders a command for terminal output.
func DisplayCommand(cmd []string) string {
	return strings.Join(cmd, " ")
}

// RedactCommand replaces known secret values before commands enter terminal events.
func RedactCommand(cmd []string, secrets []string) []string {
	if len(cmd) == 0 || len(secrets) == 0 {
		return cmd
	}
	out := append([]string(nil), cmd...)
	for i, arg := range out {
		for _, secret := range secrets {
			if secret == "" {
				continue
			}
			out[i] = strings.ReplaceAll(arg, secret, "[redacted]")
		}
	}
	return out
}
