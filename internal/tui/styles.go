package tui

import "github.com/charmbracelet/lipgloss"

var (
	// TitleStyle is the top-of-screen title bar.
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#2D6A4F")).
			Padding(0, 1)

	// StatusStyle is the status info bar beneath the title.
	StatusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#AAAAAA")).
			Background(lipgloss.Color("#1A1A2E")).
			Padding(0, 1)

	// MenuItemStyle is an unselected menu item.
	MenuItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#CCCCCC")).
			Padding(0, 2)

	// MenuSelectedStyle is the currently highlighted menu item.
	MenuSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FAFAFA")).
				Background(lipgloss.Color("#1B6CA8")).
				Padding(0, 2)

	// PanelStyle is a content panel border.
	PanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#4A90D9")).
			Padding(1, 2)

	// ErrorStyle is for error messages.
	ErrorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF4444"))

	// WarnStyle is for warning messages.
	WarnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFAA00"))

	// OkStyle is for success messages.
	OkStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#40C040"))
)
