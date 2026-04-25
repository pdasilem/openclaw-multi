package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestMenuNavigation(t *testing.T) {
	m := newMainMenu()

	// Move down.
	m2, _ := m.Update(keyText("j"))
	if m2.cursor != 1 {
		t.Errorf("expected cursor 1 after down, got %d", m2.cursor)
	}

	// Move up from start wraps to end.
	m3 := newMainMenu()
	m4, _ := m3.Update(keyText("k"))
	if m4.cursor != len(menuItems)-1 {
		t.Errorf("expected wrap to %d, got %d", len(menuItems)-1, m4.cursor)
	}
}

func TestMenuEnterDispatchesAction(t *testing.T) {
	m := newMainMenu()
	_, cmd := m.Update(keyPress(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("expected cmd after Enter")
	}
	msg := cmd()
	action, ok := msg.(MenuActionMsg)
	if !ok {
		t.Fatalf("expected MenuActionMsg, got %T", msg)
	}
	if action.ItemID != menuItems[0].id {
		t.Errorf("expected item %d, got %d", menuItems[0].id, action.ItemID)
	}
}

func TestMenuRoutesToPlaceholder(t *testing.T) {
	for _, item := range menuItems {
		m := newMainMenu()
		m.cursor = item.id - 1
		_, cmd := m.Update(keyPress(tea.Key{Code: tea.KeyEnter}))
		if cmd == nil {
			t.Fatalf("item %d: expected cmd", item.id)
		}
		msg := cmd()
		action, ok := msg.(MenuActionMsg)
		if !ok {
			t.Fatalf("item %d: expected MenuActionMsg, got %T", item.id, msg)
		}
		if action.ItemID != item.id {
			t.Errorf("item %d: got ID %d", item.id, action.ItemID)
		}
	}
}

func TestMenuWrapsAtBottom(t *testing.T) {
	m := newMainMenu()
	m.cursor = len(menuItems) - 1
	m2, _ := m.Update(keyText("j"))
	if m2.cursor != 0 {
		t.Errorf("expected wrap to 0, got %d", m2.cursor)
	}
}
