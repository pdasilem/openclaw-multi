package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
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
	_, cmd := m.Update(keyPress(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestModelUpdateQKeyOnMainMenu(t *testing.T) {
	m := newModel(nil, nil, "testhost", "")
	m.screen = screenMainMenu
	_, cmd := m.Update(keyText("q"))
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
	updated, _ := m.Update(MenuActionMsg{ItemID: 2})
	um := updated.(Model)
	if um.screen != screenPlaceholder {
		t.Errorf("expected screenPlaceholder, got %d", um.screen)
	}
}

func TestModelMenuActionRoutesToUsers(t *testing.T) {
	m := newModel(nil, nil, "testhost", "")
	m.userService = &fakeUserService{}
	m.screen = screenMainMenu
	updated, cmd := m.Update(MenuActionMsg{ItemID: 3})
	um := updated.(Model)
	if um.screen != screenUsers {
		t.Errorf("expected screenUsers, got %d", um.screen)
	}
	if cmd == nil {
		t.Fatal("expected user screen init command")
	}
}

func TestModelMenuActionRoutesToDoctor(t *testing.T) {
	m := newModel(nil, nil, "testhost", "")
	m.doctorService = &fakeDoctorService{}
	m.screen = screenMainMenu
	updated, _ := m.Update(MenuActionMsg{ItemID: 4})
	um := updated.(Model)
	if um.screen != screenDoctor {
		t.Errorf("expected screenDoctor, got %d", um.screen)
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
	if view.Content == "" {
		t.Error("expected non-empty view")
	}
}

func TestStatusBarRendersHostname(t *testing.T) {
	m := newModel(nil, nil, "vps-fra1", "")
	view := m.View()
	found := false
	for _, part := range []string{"vps-fra1"} {
		if contains(view.Content, part) {
			found = true
		}
	}
	if !found {
		t.Error("hostname not found in view")
	}
}

func keyText(text string) tea.KeyPressMsg {
	r := []rune(text)
	code := rune(0)
	if len(r) > 0 {
		code = r[0]
	}
	return tea.KeyPressMsg(tea.Key{Text: text, Code: code})
}

func keyPress(key tea.Key) tea.KeyPressMsg {
	return tea.KeyPressMsg(key)
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
