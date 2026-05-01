package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/pdasilem/openclaw-multi/internal/shell"
)

type terminalEventMsg struct {
	event shell.Event
}

type terminalModel struct {
	events       chan shell.Event
	lines        []string
	historyLimit int
	collapsed    bool
	focused      bool
	input        string
	session      *shell.PTYSession
}

func newTerminal(historyLimit int) terminalModel {
	if historyLimit <= 0 {
		historyLimit = 1000
	}
	return terminalModel{
		events:       make(chan shell.Event, 256),
		historyLimit: historyLimit,
		collapsed:    true,
	}
}

func (t terminalModel) Sink() shell.EventSink {
	return shell.ChannelSink(t.events)
}

func (t terminalModel) Init() tea.Cmd {
	return t.wait()
}

func (t terminalModel) wait() tea.Cmd {
	if t.events == nil {
		return nil
	}
	return func() tea.Msg {
		return terminalEventMsg{event: <-t.events}
	}
}

func (t terminalModel) Update(msg tea.Msg) (terminalModel, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case terminalEventMsg:
		t.appendEvent(msg.event)
		return t, t.wait(), true
	case tea.KeyMsg:
		switch strings.ToLower(msg.String()) {
		case "ctrl+t":
			t.collapsed = !t.collapsed
			return t, nil, true
		case "ctrl+g":
			t.collapsed = false
			t.focused = !t.focused
			return t, nil, true
		}
		if !t.focused {
			return t, nil, false
		}
		t.handleKey(msg)
		return t, nil, true
	}
	return t, nil, false
}

func (t *terminalModel) appendEvent(e shell.Event) {
	switch e.Kind {
	case shell.EventStart:
		t.appendLine("$ " + shell.DisplayCommand(e.Cmd))
	case shell.EventStdout, shell.EventStderr:
		for _, line := range strings.Split(strings.TrimRight(e.Data, "\n"), "\n") {
			if line != "" {
				t.appendLine(line)
			}
		}
	case shell.EventDone:
		if e.Error != "" {
			t.appendLine(fmt.Sprintf("[exit %d] %s", e.ExitCode, e.Error))
		} else {
			t.appendLine(fmt.Sprintf("[exit %d]", e.ExitCode))
		}
		t.session = nil
	}
}

func (t *terminalModel) appendLine(line string) {
	t.lines = append(t.lines, line)
	if overflow := len(t.lines) - t.historyLimit; overflow > 0 {
		t.lines = t.lines[overflow:]
	}
}

func (t *terminalModel) handleKey(msg tea.KeyMsg) {
	key := msg.String()
	if t.session != nil {
		_ = t.writeActive(key)
		return
	}
	switch key {
	case "enter":
		input := strings.TrimSpace(t.input)
		t.input = ""
		if input == "" {
			return
		}
		session, err := shell.StartPTY(context.Background(), []string{"bash", "-lc", input}, t.Sink())
		if err != nil {
			t.appendLine(err.Error())
			return
		}
		t.session = session
	case "backspace":
		if t.input != "" {
			r := []rune(t.input)
			t.input = string(r[:len(r)-1])
		}
	case "ctrl+c":
		t.input = ""
	case "esc":
		t.focused = false
	default:
		if len([]rune(key)) == 1 {
			t.input += key
		}
	}
}

func (t *terminalModel) writeActive(key string) error {
	switch key {
	case "enter":
		return t.session.Write([]byte("\r"))
	case "backspace":
		return t.session.Write([]byte{0x7f})
	case "tab":
		return t.session.Write([]byte("\t"))
	case "ctrl+c":
		return t.session.Write([]byte{0x03})
	case "up":
		return t.session.Write([]byte("\x1b[A"))
	case "down":
		return t.session.Write([]byte("\x1b[B"))
	case "right":
		return t.session.Write([]byte("\x1b[C"))
	case "left":
		return t.session.Write([]byte("\x1b[D"))
	default:
		if len([]rune(key)) == 1 {
			return t.session.Write([]byte(key))
		}
	}
	return nil
}

func (t terminalModel) View(width int) string {
	if t.collapsed {
		last := ""
		if len(t.lines) > 0 {
			last = " | " + t.lines[len(t.lines)-1]
		}
		return StatusStyle.Width(width).Render(" Terminal collapsed | Ctrl+T expand | Ctrl+G focus" + last)
	}
	var b strings.Builder
	title := " Terminal "
	if t.focused {
		title += "focused "
	}
	title += "| Ctrl+T collapse | Ctrl+G focus | Esc unfocus"
	b.WriteString(StatusStyle.Width(width).Render(title))
	b.WriteByte('\n')
	start := len(t.lines) - 8
	if start < 0 {
		start = 0
	}
	for _, line := range t.lines[start:] {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if t.session == nil {
		prompt := "$ " + t.input
		if t.focused {
			prompt += "_"
		}
		b.WriteString(prompt)
	}
	return PanelStyle.Width(width).Render(b.String())
}
