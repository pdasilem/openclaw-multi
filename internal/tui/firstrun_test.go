package tui

import "testing"

func TestFirstRunUpperQQuits(t *testing.T) {
	m := newFirstRun(nil, nil, nil)
	_, cmd := m.Update(keyText("Q"))
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}
