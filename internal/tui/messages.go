package tui

// MenuActionMsg is dispatched when the user selects a main menu item.
type MenuActionMsg struct {
	ItemID int
}

// backMsg is dispatched when the user presses Esc/q inside a sub-screen.
type backMsg struct{}
