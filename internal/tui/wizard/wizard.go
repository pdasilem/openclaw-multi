// Package wizard provides a reusable step-by-step Bubble Tea wizard model.
package wizard

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Step is implemented by each wizard step.
type Step interface {
	Name() string
	Run(ctx context.Context) error
}

// StepOutcome records the result of a completed step.
type StepOutcome struct {
	Name    string
	Skipped bool
	Err     error
}

func (o StepOutcome) OK() bool { return o.Err == nil }

// WizardDoneMsg is dispatched when all steps complete (with or without skips).
type WizardDoneMsg struct {
	Outcomes []StepOutcome
}

// state tracks where the wizard is within a step.
type stepState int

const (
	stepPending stepState = iota
	stepRunning
	stepDone
	stepFailed
)

type stepResult struct {
	index int
	err   error
}

// Model is the Bubble Tea model for a linear wizard.
type Model struct {
	ctx      context.Context
	steps    []Step
	current  int
	states   []stepState
	outcomes []StepOutcome
	errText  string // error detail for the failed step
	waitKey  bool   // true when showing retry/skip/abort prompt
	spinner  int    // tick counter for animation
	width    int
	height   int
}

// New creates a new WizardModel from a slice of steps.
func New(ctx context.Context, steps []Step) Model {
	states := make([]stepState, len(steps))
	outcomes := make([]StepOutcome, 0, len(steps))
	return Model{ctx: ctx, steps: steps, states: states, outcomes: outcomes}
}

// Init starts the first step.
func (m Model) Init() tea.Cmd {
	if len(m.steps) == 0 {
		return func() tea.Msg { return WizardDoneMsg{} }
	}
	return runStep(m.ctx, m.steps[0], 0)
}

func runStep(ctx context.Context, step Step, index int) tea.Cmd {
	return func() tea.Msg {
		err := step.Run(ctx)
		return stepResult{index: index, err: err}
	}
}

// tick animates the spinner.
type tickMsg struct{}

var _ = tickMsg{} // referenced in Update

// Update handles messages and implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case stepResult:
		if msg.index != m.current {
			return m, nil
		}
		if msg.err != nil {
			m.states[m.current] = stepFailed
			m.errText = msg.err.Error()
			m.waitKey = true
			return m, nil
		}
		m.states[m.current] = stepDone
		m.outcomes = append(m.outcomes, StepOutcome{Name: m.steps[m.current].Name()})
		m.current++
		if m.current >= len(m.steps) {
			return m, func() tea.Msg { return WizardDoneMsg{Outcomes: m.outcomes} }
		}
		m.states[m.current] = stepRunning
		return m, runStep(m.ctx, m.steps[m.current], m.current)

	case tickMsg:
		m.spinner++
		return m, nil

	case tea.KeyMsg:
		if !m.waitKey {
			return m, nil
		}
		switch strings.ToLower(msg.String()) {
		case "r":
			// Retry current step.
			m.states[m.current] = stepRunning
			m.waitKey = false
			m.errText = ""
			return m, runStep(m.ctx, m.steps[m.current], m.current)
		case "s":
			// Skip failed step and continue.
			m.outcomes = append(m.outcomes, StepOutcome{
				Name:    m.steps[m.current].Name(),
				Skipped: true,
				Err:     fmt.Errorf("skipped by user"),
			})
			m.waitKey = false
			m.errText = ""
			m.states[m.current] = stepDone
			m.current++
			if m.current >= len(m.steps) {
				return m, func() tea.Msg { return WizardDoneMsg{Outcomes: m.outcomes} }
			}
			m.states[m.current] = stepRunning
			return m, runStep(m.ctx, m.steps[m.current], m.current)
		case "a", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

var (
	spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	okStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#40C040")).Bold(true)
	failStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4444")).Bold(true)
	warnStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFAA00"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
)

// View renders the wizard.
func (m Model) View() string {
	var b strings.Builder
	total := len(m.steps)

	for i, step := range m.steps {
		switch m.states[i] {
		case stepPending:
			fmt.Fprintf(&b, "  %s  %s\n", dimStyle.Render("○"), dimStyle.Render(step.Name()))
		case stepRunning:
			frame := spinnerFrames[m.spinner%len(spinnerFrames)]
			fmt.Fprintf(&b, "  %s  [%d/%d] %s...\n", frame, i+1, total, step.Name())
		case stepDone:
			fmt.Fprintf(&b, "  %s  %s\n", okStyle.Render("✓"), step.Name())
		case stepFailed:
			fmt.Fprintf(&b, "  %s  %s\n", failStyle.Render("✗"), step.Name())
		}
	}

	if m.waitKey && m.errText != "" {
		b.WriteString("\n")
		b.WriteString(failStyle.Render("  Error: ") + m.errText + "\n\n")
		b.WriteString(warnStyle.Render("  [R]etry  [S]kip  [A]bort") + "\n")
	}

	return b.String()
}
