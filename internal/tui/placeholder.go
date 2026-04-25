package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// placeholderModel shows the "not implemented yet" screen for a menu item.
type placeholderModel struct {
	itemID int
	title  string
	width  int
	height int
}

func newPlaceholder(itemID int, title string) placeholderModel {
	return placeholderModel{itemID: itemID, title: title}
}

func (p placeholderModel) Init() tea.Cmd { return nil }

func (p placeholderModel) Update(msg tea.Msg) (placeholderModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return p, func() tea.Msg { return backMsg{} }
		}
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
	}
	return p, nil
}

func (p placeholderModel) View() string {
	phase, ok := menuPhases[p.itemID]
	phaseStr := "?"
	if ok {
		phaseStr = fmt.Sprintf("%d", phase)
	}
	content := strings.Join([]string{
		"",
		fmt.Sprintf("  Not implemented yet (phase %s).", phaseStr),
		"",
		"  See OPENCLAW_OVERLAY_PLAN_RU.md §6." + fmt.Sprintf("%d", p.itemID) +
			" for the planned UI.",
		"",
		"  Press Esc or q to return to the main menu.",
		"",
	}, "\n")
	return PanelStyle.Width(60).Render(
		TitleStyle.Render(fmt.Sprintf(" %s ", p.title)) + "\n" + content,
	)
}
