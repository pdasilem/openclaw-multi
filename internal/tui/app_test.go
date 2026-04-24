package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestModelInitReturnsNilCmd(t *testing.T) {
	m := newModel(nil, nil, "testhost", "")
	cmd := m.Init()
	if cmd != nil {
		t.Error("expected nil Cmd from Init")
	}
}

func TestModelUpdateQuit(t *testing.T) {
	m := newModel(nil, nil, "testhost", "")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestModelUpdateQKeyOnMainMenu(t *testing.T) {
	m := newModel(nil, nil, "testhost", "")
	m.screen = screenMainMenu
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Error("expected quit cmd when pressing q on main menu")
	}
}

func TestModelUpdateWindowSize(t *testing.T) {
	m := newModel(nil, nil, "testhost", "")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	um := updated.(Model)
	if um.width != 120 || um.height != 40 {
		t.Errorf("window size not updated: got %dx%d", um.width, um.height)
	}
}

func TestModelMenuActionRoutesToPlaceholder(t *testing.T) {
	m := newModel(nil, nil, "testhost", "")
	m.screen = screenMainMenu
	updated, _ := m.Update(MenuActionMsg{ItemID: 3})
	um := updated.(Model)
	if um.screen != screenPlaceholder {
		t.Errorf("expected screenPlaceholder, got %d", um.screen)
	}
}

func TestModelBackReturnsToMainMenu(t *testing.T) {
	m := newModel(nil, nil, "testhost", "")
	m.screen = screenPlaceholder
	updated, _ := m.Update(backMsg{})
	um := updated.(Model)
	if um.screen != screenMainMenu {
		t.Errorf("expected screenMainMenu after back, got %d", um.screen)
	}
}

func TestModelViewRendersHostname(t *testing.T) {
	m := newModel(nil, nil, "myhostname", "")
	view := m.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
}

func TestStatusBarRendersHostname(t *testing.T) {
	m := newModel(nil, nil, "vps-fra1", "")
	view := m.View()
	found := false
	for _, part := range []string{"vps-fra1"} {
		if contains(view, part) {
			found = true
		}
	}
	if !found {
		t.Error("hostname not found in view")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
