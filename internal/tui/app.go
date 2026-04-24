// Package tui implements the Bubble Tea TUI for openclaw-multi.
package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SergeiM/openclaw-multi/internal/admin"
	"github.com/SergeiM/openclaw-multi/internal/audit"
	"github.com/SergeiM/openclaw-multi/internal/state"
)

const version = "v0.0.0"

// screen identifies which sub-screen is active.
type screen int

const (
	screenMainMenu screen = iota
	screenPlaceholder
	screenFirstRun
)

// Model is the root Bubble Tea model.
type Model struct {
	width, height int
	hostname      string
	rootWarn      string

	screen      screen
	menu        mainMenuModel
	placeholder placeholderModel
	firstRun    firstRunModel

	store  *state.Store
	logger *audit.Logger
}

func newModel(store *state.Store, logger *audit.Logger, hostname, rootWarn string) Model {
	return Model{
		screen:   screenMainMenu,
		menu:     newMainMenu(),
		store:    store,
		logger:   logger,
		hostname: hostname,
		rootWarn: rootWarn,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if m.screen == screenMainMenu {
				return m, tea.Quit
			}
		}

	case MenuActionMsg:
		title := menuItems[msg.ItemID-1].label
		m.placeholder = newPlaceholder(msg.ItemID, title)
		m.screen = screenPlaceholder
		return m, nil

	case backMsg:
		m.screen = screenMainMenu
		return m, nil

	case firstRunDoneMsg:
		m.screen = screenMainMenu
		return m, nil
	}

	switch m.screen {
	case screenMainMenu:
		updated, cmd := m.menu.Update(msg)
		m.menu = updated
		return m, cmd
	case screenPlaceholder:
		updated, cmd := m.placeholder.Update(msg)
		m.placeholder = updated
		return m, cmd
	case screenFirstRun:
		updated, cmd := m.firstRun.Update(msg)
		m.firstRun = updated
		return m, cmd
	}
	return m, nil
}

// View implements tea.Model.
func (m Model) View() string {
	var b strings.Builder

	// Title bar.
	title := fmt.Sprintf(" OpenClaw Multi-User Overlay %s ", version)
	b.WriteString(TitleStyle.Width(m.width).Render(title))
	b.WriteByte('\n')

	// Status bar.
	status := fmt.Sprintf(" Host: %s  |  Tailscale: ?  |  CF Tunnel: ?  |  Users: 0 ",
		m.hostname)
	b.WriteString(StatusStyle.Width(m.width).Render(status))
	b.WriteByte('\n')

	// Root warning (if applicable).
	if m.rootWarn != "" {
		b.WriteString(WarnStyle.Render(m.rootWarn))
		b.WriteByte('\n')
	}

	b.WriteByte('\n')

	switch m.screen {
	case screenMainMenu:
		b.WriteString(m.menu.View())
	case screenPlaceholder:
		b.WriteString(m.placeholder.View())
	case screenFirstRun:
		b.WriteString(m.firstRun.View())
	}

	b.WriteByte('\n')
	b.WriteString(StatusStyle.Render("  ↑/↓ navigate   Enter select   q quit"))
	return b.String()
}

// Run is the main entrypoint for the TUI binary.
func Run() error {
	ctx := context.Background()

	stateDir := os.Getenv("OPENCLAW_STATE_DIR")
	statePath := ""
	if stateDir != "" {
		statePath = stateDir + "/state.db"
	}
	store, err := state.Open(ctx, statePath)
	if err != nil {
		return fmt.Errorf("open state: %w", err)
	}
	defer func() { _ = store.Close() }()

	logPath := os.Getenv("OPENCLAW_AUDIT_LOG")
	logger, err := audit.New(logPath)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer func() { _ = logger.Close() }()

	hostname, _ := os.Hostname()
	rootWarn := admin.WarnIfRoot()

	existing, err := store.GetAdmin(ctx)

	var m Model
	switch {
	case errors.Is(err, state.ErrNoAdmin):
		candidate, cerr := admin.ResolveCandidate()
		if cerr != nil {
			candidate = &admin.User{Username: "unknown"}
		}
		fr := newFirstRun(store, logger, candidate)
		m = newModel(store, logger, hostname, rootWarn)
		m.screen = screenFirstRun
		m.firstRun = fr
	case err != nil:
		return fmt.Errorf("read admin: %w", err)
	default:
		_ = existing
		current, err := admin.ResolveCurrent()
		if err != nil {
			return fmt.Errorf("resolve current user: %w", err)
		}
		if verr := admin.VerifyAdmin(ctx, store, current); verr != nil {
			_ = logger.Emit(audit.Event{
				Actor:        current.Username,
				Action:       audit.ActionStartup,
				Target:       "system",
				Result:       audit.ResultError,
				ErrorMessage: verr.Error(),
			})
			return fmt.Errorf("access denied: %w", verr)
		}
		_ = logger.Emit(audit.Event{
			Actor:  current.Username,
			Action: audit.ActionStartup,
			Target: "system",
			Result: audit.ResultOk,
		})
		m = newModel(store, logger, hostname, rootWarn)
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
