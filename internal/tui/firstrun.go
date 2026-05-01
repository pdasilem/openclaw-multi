package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/pdasilem/openclaw-multi/internal/admin"
	"github.com/pdasilem/openclaw-multi/internal/audit"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

// firstRunDoneMsg signals that first-run setup completed successfully.
type firstRunDoneMsg struct{}

// firstRunModel manages the first-run admin identification screen.
type firstRunModel struct {
	store     *state.Store
	logger    *audit.Logger
	candidate *admin.User
	err       string
}

func newFirstRun(store *state.Store, logger *audit.Logger, candidate *admin.User) firstRunModel {
	return firstRunModel{store: store, logger: logger, candidate: candidate}
}

func (f firstRunModel) Init() tea.Cmd { return nil }

func (f firstRunModel) Update(msg tea.Msg) (firstRunModel, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return f, nil
	}
	switch strings.ToLower(km.String()) {
	case "y", "enter":
		if err := f.persistAdmin(); err != nil {
			f.err = err.Error()
			return f, nil
		}
		return f, func() tea.Msg { return firstRunDoneMsg{} }
	case "ctrl+c", "q", "esc":
		return f, tea.Quit
	}
	return f, nil
}

func (f *firstRunModel) persistAdmin() error {
	if f.candidate == nil || f.candidate.Username == "" {
		return fmt.Errorf("no candidate user resolved")
	}
	ctx := context.Background()
	if err := admin.PersistAdmin(ctx, f.store, f.logger, f.candidate, "first-run"); err != nil {
		return fmt.Errorf("save admin: %w", err)
	}
	return nil
}

func (f firstRunModel) View() string {
	var b strings.Builder
	b.WriteString(TitleStyle.Render(" First-run: Admin Setup "))
	b.WriteString("\n\n")

	username := "unknown"
	if f.candidate != nil {
		username = f.candidate.Username
	}

	b.WriteString("  Detected admin candidate: " + OkStyle.Render(username) + "\n\n")
	b.WriteString("  Save this account as the overlay administrator?\n\n")
	b.WriteString("  Press Y / Enter to confirm, q to quit.\n")

	if f.err != "" {
		b.WriteString("\n")
		b.WriteString(ErrorStyle.Render("  Error: " + f.err))
	}

	return PanelStyle.Width(60).Render(b.String())
}
