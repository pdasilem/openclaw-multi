package shell

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
)

// PTYSession is an active interactive command attached to a pseudo-terminal.
type PTYSession struct {
	cmd    *exec.Cmd
	file   *os.File
	events EventSink
	done   chan error
	once   sync.Once
}

// StartPTY starts cmd with a pseudo-terminal and streams output to events.
func StartPTY(ctx context.Context, cmd []string, events EventSink) (*PTYSession, error) {
	if len(cmd) == 0 {
		return nil, fmt.Errorf("pty command is empty")
	}
	started := time.Now().UTC()
	c := exec.CommandContext(ctx, cmd[0], cmd[1:]...) //nolint:gosec
	f, err := pty.Start(c)
	if err != nil {
		return nil, fmt.Errorf("start pty command %q: %w", cmd[0], err)
	}
	s := &PTYSession{cmd: c, file: f, events: events, done: make(chan error, 1)}
	emit(events, Event{Kind: EventStart, Cmd: cmd, StartedAt: started})
	go s.read(cmd)
	go s.wait(cmd, started)
	return s, nil
}

// Write sends bytes to the PTY.
func (s *PTYSession) Write(b []byte) error {
	if s == nil || s.file == nil {
		return fmt.Errorf("pty session is not active")
	}
	_, err := s.file.Write(b)
	return err
}

// Resize changes PTY size.
func (s *PTYSession) Resize(cols, rows uint16) error {
	if s == nil || s.file == nil {
		return nil
	}
	return pty.Setsize(s.file, &pty.Winsize{Cols: cols, Rows: rows})
}

// Close terminates the PTY session.
func (s *PTYSession) Close() error {
	if s == nil {
		return nil
	}
	var err error
	s.once.Do(func() {
		if s.file != nil {
			err = s.file.Close()
		}
	})
	return err
}

func (s *PTYSession) Done() <-chan error { return s.done }

func (s *PTYSession) read(cmd []string) {
	r := bufio.NewReader(s.file)
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			emit(s.events, Event{Kind: EventStdout, Cmd: cmd, Data: string(buf[:n])})
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				emit(s.events, Event{Kind: EventStderr, Cmd: cmd, Data: err.Error()})
			}
			return
		}
	}
}

func (s *PTYSession) wait(cmd []string, started time.Time) {
	err := s.cmd.Wait()
	exitCode := 0
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			exitCode = -1
		}
	}
	_ = s.Close()
	emit(s.events, Event{
		Kind:       EventDone,
		Cmd:        cmd,
		ExitCode:   exitCode,
		Error:      errMsg,
		StartedAt:  started,
		DurationMs: time.Since(started).Milliseconds(),
	})
	s.done <- err
	close(s.done)
}

func emit(sink EventSink, e Event) {
	if sink != nil {
		sink.EmitShellEvent(e)
	}
}
