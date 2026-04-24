package wizard

import (
	"context"
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// mockStep is a Step that returns a pre-configured error (or nil).
type mockStep struct {
	name string
	err  error
}

func (s *mockStep) Name() string                { return s.name }
func (s *mockStep) Run(_ context.Context) error { return s.err }

// update is a helper that runs Update and casts the result back to Model.
func update(m Model, msg tea.Msg) (Model, tea.Cmd) {
	newModel, cmd := m.Update(msg)
	return newModel.(Model), cmd
}

func TestWizardAdvancesOnSuccess(t *testing.T) {
	steps := []Step{
		&mockStep{name: "step1"},
		&mockStep{name: "step2"},
		&mockStep{name: "step3"},
	}
	m := New(context.Background(), steps)
	cmd := m.Init()

	for range 3 {
		if cmd == nil {
			break
		}
		msg := cmd()
		m, cmd = update(m, msg)
	}

	if cmd == nil {
		t.Fatal("expected WizardDoneMsg cmd")
	}
	done, ok := cmd().(WizardDoneMsg)
	if !ok {
		t.Fatalf("expected WizardDoneMsg, got %T", cmd())
	}
	if len(done.Outcomes) != 3 {
		t.Errorf("expected 3 outcomes, got %d", len(done.Outcomes))
	}
}

func TestWizardShowsErrorOnFailure(t *testing.T) {
	steps := []Step{
		&mockStep{name: "fail-step", err: errors.New("something broke")},
	}
	m := New(context.Background(), steps)
	cmd := m.Init()
	m, _ = update(m, cmd())

	if !m.waitKey {
		t.Error("expected waitKey=true after step failure")
	}
	if m.errText != "something broke" {
		t.Errorf("errText: got %q", m.errText)
	}
	if m.View() == "" {
		t.Error("expected non-empty view")
	}
}

func TestWizardRetry(t *testing.T) {
	callCount := 0
	step := &callCountStep{name: "retry-step", maxFail: 1, count: &callCount}
	m := New(context.Background(), []Step{step})

	// First run — fails.
	cmd := m.Init()
	m, _ = update(m, cmd())
	if !m.waitKey {
		t.Fatal("expected waitKey=true after failure")
	}

	// Press R to retry.
	m, cmd = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd != nil {
		m, cmd = update(m, cmd())
	}
	if cmd == nil {
		t.Fatal("expected cmd after retry success")
	}
	if _, ok := cmd().(WizardDoneMsg); !ok {
		t.Errorf("expected WizardDoneMsg after retry")
	}
}

func TestWizardSkip(t *testing.T) {
	steps := []Step{
		&mockStep{name: "fail", err: errors.New("oops")},
		&mockStep{name: "ok"},
	}
	m := New(context.Background(), steps)
	cmd := m.Init()
	m, _ = update(m, cmd())

	// Skip the failed step.
	m, cmd = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if cmd != nil {
		_, cmd = update(m, cmd())
	}
	if cmd == nil {
		t.Fatal("expected WizardDoneMsg after skip+ok")
	}
	done, ok := cmd().(WizardDoneMsg)
	if !ok {
		t.Fatalf("expected WizardDoneMsg")
	}
	if len(done.Outcomes) != 2 {
		t.Errorf("expected 2 outcomes, got %d", len(done.Outcomes))
	}
	if !done.Outcomes[0].Skipped {
		t.Error("first outcome should be Skipped")
	}
	if done.Outcomes[1].Skipped {
		t.Error("second outcome should not be Skipped")
	}
}

func TestWizardAbort(t *testing.T) {
	steps := []Step{&mockStep{name: "fail", err: errors.New("fail")}}
	m := New(context.Background(), steps)
	cmd := m.Init()
	m, _ = update(m, cmd())

	_, cmd = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if cmd == nil {
		t.Fatal("expected quit cmd after Abort")
	}
}

func TestWizardEmptySteps(t *testing.T) {
	m := New(context.Background(), nil)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected WizardDoneMsg for empty steps")
	}
	if _, ok := cmd().(WizardDoneMsg); !ok {
		t.Error("expected WizardDoneMsg for empty step list")
	}
}

func TestWizardWindowResize(t *testing.T) {
	m := New(context.Background(), []Step{&mockStep{name: "s"}})
	m, _ = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.width != 120 || m.height != 40 {
		t.Errorf("window size not updated: %dx%d", m.width, m.height)
	}
}

// callCountStep fails the first maxFail calls then succeeds.
type callCountStep struct {
	name    string
	maxFail int
	count   *int
}

func (s *callCountStep) Name() string { return s.name }
func (s *callCountStep) Run(_ context.Context) error {
	*s.count++
	if *s.count <= s.maxFail {
		return errors.New("transient failure")
	}
	return nil
}

func TestWizardKeyIgnoredWhenNotWaiting(t *testing.T) {
	// Key press should be ignored when wizard is not in waitKey state
	m := New(context.Background(), []Step{&mockStep{name: "pending"}})
	// Don't init — just send a key press, should be ignored
	m2, cmd := update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd != nil {
		t.Error("expected nil cmd when not in waitKey state")
	}
	if m2.waitKey {
		t.Error("unexpected waitKey=true")
	}
}

func TestWizardViewAllStates(t *testing.T) {
	steps := []Step{
		&mockStep{name: "pending"},
		&mockStep{name: "done"},
	}
	m := New(context.Background(), steps)
	// Init starts first step running
	cmd := m.Init()
	_ = cmd
	view := m.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
}
