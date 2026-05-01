// Package tui implements the Bubble Tea TUI for openclaw-multi.
package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/pdasilem/openclaw-multi/internal/admin"
	"github.com/pdasilem/openclaw-multi/internal/api"
	"github.com/pdasilem/openclaw-multi/internal/audit"
	"github.com/pdasilem/openclaw-multi/internal/backup"
	"github.com/pdasilem/openclaw-multi/internal/config"
	"github.com/pdasilem/openclaw-multi/internal/doctor"
	"github.com/pdasilem/openclaw-multi/internal/network"
	"github.com/pdasilem/openclaw-multi/internal/shell"
	"github.com/pdasilem/openclaw-multi/internal/state"
	"github.com/pdasilem/openclaw-multi/internal/tui/wizard"
	userops "github.com/pdasilem/openclaw-multi/internal/users"
)

const version = "v0.1.0"

// screen identifies which sub-screen is active.
type screen int

const (
	screenMainMenu screen = iota
	screenPlaceholder
	screenFirstRun
	screenWizard
	screenUsers
	screenDoctor
	screenNetwork
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
	wizard      tea.Model
	users       userManagementModel
	doctor      doctorModel
	network     networkModel
	terminal    terminalModel

	store          *state.Store
	logger         *audit.Logger
	exec           shell.Executor
	fs             shell.FS
	cfg            *config.OverlayConfig
	userService    userService
	backupService  backupService
	doctorService  doctorService
	networkService networkService
}

func newModel(store *state.Store, logger *audit.Logger, hostname string, cfg *config.OverlayConfig, exec shell.Executor, fs shell.FS, terminal terminalModel) Model {
	return Model{
		screen:   screenMainMenu,
		menu:     newMainMenu(),
		store:    store,
		logger:   logger,
		hostname: hostname,
		exec:     exec,
		fs:       fs,
		cfg:      cfg,
		terminal: terminal,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return m.terminal.Init() }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		if updated, cmd, handled := m.terminal.Update(msg); handled {
			m.terminal = updated
			return m, cmd
		}
		switch strings.ToLower(msg.String()) {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if m.screen == screenMainMenu {
				return m, tea.Quit
			}
		}

	case MenuActionMsg:
		if msg.ItemID == 1 {
			// Menu item 1 — launch fresh-install wizard.
			wiz := launchFreshInstall(context.Background(), m.store, m.logger, m.exec, m.fs, m.cfg)
			m.wizard = wiz
			m.screen = screenWizard
			return m, wiz.Init()
		}
		if msg.ItemID == 3 {
			m.users = newUserManagement(m.userService, m.backupService)
			m.screen = screenUsers
			return m, m.users.Init()
		}
		if msg.ItemID == 4 {
			m.doctor = newDoctor(m.doctorService)
			m.screen = screenDoctor
			return m, m.doctor.Init()
		}
		if msg.ItemID == 5 {
			m.network = newNetwork(m.networkService)
			m.screen = screenNetwork
			return m, m.network.Init()
		}
		title := menuItems[msg.ItemID-1].label
		m.placeholder = newPlaceholder(msg.ItemID, title)
		m.screen = screenPlaceholder
		return m, nil

	case wizard.WizardDoneMsg:
		m.screen = screenMainMenu
		return m, nil

	case backMsg:
		m.screen = screenMainMenu
		return m, nil

	case firstRunDoneMsg:
		m.screen = screenMainMenu
		return m, nil
	case terminalEventMsg:
		updated, cmd, _ := m.terminal.Update(msg)
		m.terminal = updated
		return m, cmd
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
	case screenWizard:
		if m.wizard != nil {
			updated, cmd := m.wizard.Update(msg)
			m.wizard = updated
			return m, cmd
		}
	case screenUsers:
		updated, cmd := m.users.Update(msg)
		m.users = updated
		return m, cmd
	case screenDoctor:
		updated, cmd := m.doctor.Update(msg)
		m.doctor = updated
		return m, cmd
	case screenNetwork:
		updated, cmd := m.network.Update(msg)
		m.network = updated
		return m, cmd
	}
	return m, nil
}

// View implements tea.Model.
func (m Model) View() tea.View {
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
	case screenWizard:
		if m.wizard != nil {
			b.WriteString(m.wizard.View().Content)
		}
	case screenUsers:
		b.WriteString(m.users.View())
	case screenDoctor:
		b.WriteString(m.doctor.View())
	case screenNetwork:
		b.WriteString(m.network.View())
	}

	b.WriteByte('\n')
	b.WriteString(StatusStyle.Render("  ↑/↓ navigate   Enter select   q quit"))
	b.WriteByte('\n')
	b.WriteString(m.terminal.View(m.width))
	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// launchFreshInstall creates the fresh-install wizard model.
func launchFreshInstall(ctx context.Context, store *state.Store, logger *audit.Logger, exec shell.Executor, fs shell.FS, cfg *config.OverlayConfig) tea.Model {
	d := wizard.Deps{
		Exec:        exec,
		FS:          fs,
		Logger:      logger,
		Store:       store,
		Cfg:         cfg,
		CfgPath:     "/etc/openclaw-multi/config.yml",
		TmplDir:     "/opt/openclaw-multi/templates",
		Interactive: true,
	}
	steps := wizard.NewFreshInstallSteps(d)
	return wizard.New(ctx, steps)
}

// Run is the main entrypoint for the TUI binary.
func Run() error {
	ctx := context.Background()
	if admin.IsRoot() {
		return fmt.Errorf("openclaw-multi must run as the configured admin user without sudo; run sudo openclaw-multi system-prepare once for system preparation")
	}
	current, err := admin.ResolveCurrent()
	if err != nil {
		return fmt.Errorf("resolve current user: %w", err)
	}

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
	cfg, cfgErr := config.Load("/etc/openclaw-multi/config.yml")
	if cfgErr != nil && !errors.Is(cfgErr, config.ErrNotFound) {
		return fmt.Errorf("load config: %w", cfgErr)
	}
	terminal := newTerminal(cfg.TerminalHistoryLines)
	exec := &shell.RealExecutor{
		Logger: logger,
		Events: terminal.Sink(),
		Actor:  current.Username,
		Redact: terminalSecrets(cfg),
	}
	fs := shell.PrivilegedFS{Base: shell.RealFS{}, Exec: exec}
	socketPath := os.Getenv("OPENCLAW_OVERLAY_SOCKET")
	if socketPath == "" {
		socketPath = api.DefaultSocketPath
	}
	userManager := userops.NewManager(store, exec, fs, cfg, api.Client{SocketPath: socketPath}, logger)
	userManager.TemplateDir = "/opt/openclaw-multi/templates"
	backupManager := backup.NewManager(store, exec, fs, logger, backup.Options{TemplateDir: "/opt/openclaw-multi/templates"})
	userManager.BeforeRemove = backupManager
	doctorChecker := doctor.NewChecker(store, exec, fs, logger, doctor.Options{})
	networkManager := network.NewManager(store, exec, cfg, nil, logger)

	existing, err := store.GetAdmin(ctx)

	var m Model
	switch {
	case errors.Is(err, state.ErrNoAdmin):
		fr := newFirstRun(store, logger, current)
		m = newModel(store, logger, hostname, cfg, exec, fs, terminal)
		m.userService = userManager
		m.backupService = backupManager
		m.doctorService = doctorChecker
		m.networkService = networkManager
		m.screen = screenFirstRun
		m.firstRun = fr
	case err != nil:
		return fmt.Errorf("read admin: %w", err)
	default:
		_ = existing
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
		m = newModel(store, logger, hostname, cfg, exec, fs, terminal)
		m.userService = userManager
		m.backupService = backupManager
		m.doctorService = doctorChecker
		m.networkService = networkManager
	}

	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}

func terminalSecrets(cfg *config.OverlayConfig) []string {
	if cfg == nil {
		return nil
	}
	return []string{
		cfg.CloudflareAPIToken,
		cfg.Notifications.TelegramToken,
		cfg.Notifications.TelegramChatID,
	}
}
