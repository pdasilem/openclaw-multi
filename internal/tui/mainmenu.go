package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// menuItem represents one entry in the main menu.
type menuItem struct {
	id    int
	label string
}

// menuItems is the ordered list of main menu entries (§6.1 master plan).
var menuItems = []menuItem{
	{1, "Fresh install"},
	{2, "Update OpenClaw"},
	{3, "User management"},
	{4, "Health check / Doctor"},
	{5, "Network and firewall (Tailscale, Cloudflare, UFW)"},
	{6, "Plugins and tunnels (user overview)"},
	{7, "Logs and monitoring"},
	{8, "Security audit"},
	{9, "Diagnostic snapshot (support)"},
	{10, "Remove OpenClaw / overlay"},
}

// menuPhases maps menu item ID → the implementation phase number.
var menuPhases = map[int]int{
	1: 1, 2: 1, 3: 2, 4: 4, 5: 5,
	6: 7, 7: 8, 8: 9, 9: 10, 10: 11,
}

// mainMenuModel is the Bubble Tea model for the main menu.
type mainMenuModel struct {
	cursor int
	width  int
	height int
}

func newMainMenu() mainMenuModel {
	return mainMenuModel{}
}

func (m mainMenuModel) Init() tea.Cmd { return nil }

func (m mainMenuModel) Update(msg tea.Msg) (mainMenuModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			} else {
				m.cursor = len(menuItems) - 1
			}
		case "down", "j":
			if m.cursor < len(menuItems)-1 {
				m.cursor++
			} else {
				m.cursor = 0
			}
		case "enter", " ":
			return m, func() tea.Msg {
				return MenuActionMsg{ItemID: menuItems[m.cursor].id}
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m mainMenuModel) View() string {
	var b strings.Builder
	for i, item := range menuItems {
		line := fmt.Sprintf("  %2d. %s", item.id, item.label)
		if i == m.cursor {
			b.WriteString(MenuSelectedStyle.Render(line))
		} else {
			b.WriteString(MenuItemStyle.Render(line))
		}
		b.WriteByte('\n')
	}
	b.WriteString(MenuItemStyle.Render("   0. Exit"))
	return b.String()
}
