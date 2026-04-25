package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	backupops "github.com/pdasilem/openclaw-multi/internal/backup"
	"github.com/pdasilem/openclaw-multi/internal/state"
	userops "github.com/pdasilem/openclaw-multi/internal/users"
)

type userService interface {
	List(ctx context.Context) ([]state.User, error)
	Add(ctx context.Context, req userops.AddRequest) (*state.User, error)
	Deactivate(ctx context.Context, username string) error
	Activate(ctx context.Context, username string) error
	Remove(ctx context.Context, req userops.RemoveRequest) error
}

type backupService interface {
	List(ctx context.Context, username string) ([]state.Backup, error)
	Create(ctx context.Context, req backupops.CreateRequest) (*state.Backup, error)
	Restore(ctx context.Context, req backupops.RestoreRequest) error
}

type userMode int

const (
	userModeList userMode = iota
	userModeAdd
	userModeRemove
	userModeRestore
)

type userManagementModel struct {
	service      userService
	backups      backupService
	users        []state.User
	backupList   []state.Backup
	cursor       int
	backupCursor int
	mode         userMode
	input        string
	status       string
	errText      string
}

type usersLoadedMsg struct {
	users []state.User
	err   error
}

type userOpDoneMsg struct {
	status string
	err    error
}

type backupsLoadedMsg struct {
	backups []state.Backup
	err     error
}

func newUserManagement(service userService, backups backupService) userManagementModel {
	return userManagementModel{service: service, backups: backups}
}

func (m userManagementModel) Init() tea.Cmd {
	return m.loadCmd()
}

func (m userManagementModel) Update(msg tea.Msg) (userManagementModel, tea.Cmd) {
	switch msg := msg.(type) {
	case usersLoadedMsg:
		m.users = msg.users
		m.errText = errString(msg.err)
		if m.cursor >= len(m.users) {
			m.cursor = max(0, len(m.users)-1)
		}
		return m, nil
	case userOpDoneMsg:
		m.errText = errString(msg.err)
		if msg.err == nil {
			m.status = msg.status
			m.mode = userModeList
			m.input = ""
			return m, m.loadCmd()
		}
		return m, nil
	case backupsLoadedMsg:
		m.backupList = msg.backups
		m.backupCursor = 0
		m.errText = errString(msg.err)
		if msg.err == nil {
			m.mode = userModeRestore
		}
		return m, nil
	case tea.KeyMsg:
		switch m.mode {
		case userModeAdd:
			return m.updateAdd(msg)
		case userModeRemove:
			return m.updateRemove(msg)
		case userModeRestore:
			return m.updateRestore(msg)
		default:
			return m.updateList(msg)
		}
	}
	return m, nil
}

func (m userManagementModel) View() string {
	var b strings.Builder
	b.WriteString("User management\n\n")
	if m.errText != "" {
		b.WriteString(WarnStyle.Render("Error: " + m.errText))
		b.WriteString("\n\n")
	}
	if m.status != "" {
		b.WriteString(StatusStyle.Render(m.status))
		b.WriteString("\n\n")
	}

	switch m.mode {
	case userModeAdd:
		b.WriteString("Add user. Domain/subdomain must already be configured.\n")
		b.WriteString("Username: " + m.input + "\n\n")
		b.WriteString("Enter add   Esc cancel")
		return b.String()
	case userModeRemove:
		user := m.selectedUser()
		b.WriteString(WarnStyle.Render("Remove is destructive. Backup is Phase 3."))
		b.WriteString("\n")
		if user != nil && user.Status == state.UserStatusActive {
			b.WriteString("Active user will be deactivated first, then deleted.\n")
		}
		b.WriteString("Type username to confirm: " + m.input + "\n\n")
		b.WriteString("Enter remove   Esc cancel")
		return b.String()
	case userModeRestore:
		user := m.selectedUser()
		if user == nil {
			return "No selected user.\n\nEsc cancel"
		}
		b.WriteString("Restore backup for " + user.Username + "\n\n")
		if len(m.backupList) == 0 {
			b.WriteString("No backups recorded for this user.\n\nEsc cancel")
			return b.String()
		}
		for i, backup := range m.backupList {
			line := fmt.Sprintf("  %s  %d bytes  %s", backup.ID, backup.SizeBytes, backup.Path)
			if i == m.backupCursor {
				b.WriteString(MenuSelectedStyle.Render(line))
			} else {
				b.WriteString(MenuItemStyle.Render(line))
			}
			b.WriteByte('\n')
		}
		b.WriteString("\nEnter restore   Esc cancel")
		return b.String()
	}

	if len(m.users) == 0 {
		b.WriteString("No managed users yet.\n\n")
		b.WriteString("a add user   q back")
		return b.String()
	}
	for i, u := range m.users {
		line := fmt.Sprintf("  %-16s %-6s port=%d  %s", u.Username, u.Status, u.Port, u.GatewayURL)
		if i == m.cursor {
			b.WriteString(MenuSelectedStyle.Render(line))
		} else {
			b.WriteString(MenuItemStyle.Render(line))
		}
		b.WriteByte('\n')
	}
	b.WriteString("\n↑/↓ navigate   a add   b backup   r restore   p activate/deactivate   x remove   q back")
	return b.String()
}

func (m userManagementModel) updateList(msg tea.KeyMsg) (userManagementModel, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		return m, func() tea.Msg { return backMsg{} }
	case "up", "k":
		if len(m.users) > 0 {
			if m.cursor > 0 {
				m.cursor--
			} else {
				m.cursor = len(m.users) - 1
			}
		}
	case "down", "j":
		if len(m.users) > 0 {
			if m.cursor < len(m.users)-1 {
				m.cursor++
			} else {
				m.cursor = 0
			}
		}
	case "a":
		m.mode = userModeAdd
		m.input = ""
		m.errText = ""
	case "p":
		user := m.selectedUser()
		if user == nil {
			return m, nil
		}
		return m, m.toggleCmd(*user)
	case "b":
		user := m.selectedUser()
		if user == nil {
			return m, nil
		}
		return m, m.backupCmd(*user)
	case "r":
		user := m.selectedUser()
		if user == nil {
			return m, nil
		}
		return m, m.loadBackupsCmd(*user)
	case "x":
		if m.selectedUser() != nil {
			m.mode = userModeRemove
			m.input = ""
			m.errText = ""
		}
	}
	return m, nil
}

func (m userManagementModel) updateRestore(msg tea.KeyMsg) (userManagementModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = userModeList
		m.errText = ""
	case "up", "k":
		if len(m.backupList) > 0 {
			if m.backupCursor > 0 {
				m.backupCursor--
			} else {
				m.backupCursor = len(m.backupList) - 1
			}
		}
	case "down", "j":
		if len(m.backupList) > 0 {
			if m.backupCursor < len(m.backupList)-1 {
				m.backupCursor++
			} else {
				m.backupCursor = 0
			}
		}
	case "enter":
		user := m.selectedUser()
		if user == nil || len(m.backupList) == 0 {
			return m, nil
		}
		return m, m.restoreCmd(*user, m.backupList[m.backupCursor])
	}
	return m, nil
}

func (m userManagementModel) updateAdd(msg tea.KeyMsg) (userManagementModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = userModeList
		m.input = ""
		m.errText = ""
	case "enter":
		username := strings.TrimSpace(m.input)
		return m, m.addCmd(username)
	case "backspace":
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
	default:
		m.input += printableKey(msg)
	}
	return m, nil
}

func (m userManagementModel) updateRemove(msg tea.KeyMsg) (userManagementModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = userModeList
		m.input = ""
		m.errText = ""
	case "enter":
		user := m.selectedUser()
		if user == nil {
			return m, nil
		}
		if m.input != user.Username {
			m.errText = "confirmation must match username exactly"
			return m, nil
		}
		return m, m.removeCmd(*user)
	case "backspace":
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
	default:
		m.input += printableKey(msg)
	}
	return m, nil
}

func (m userManagementModel) selectedUser() *state.User {
	if m.cursor < 0 || m.cursor >= len(m.users) {
		return nil
	}
	return &m.users[m.cursor]
}

func (m userManagementModel) loadCmd() tea.Cmd {
	return func() tea.Msg {
		if m.service == nil {
			return usersLoadedMsg{err: fmt.Errorf("user service unavailable")}
		}
		users, err := m.service.List(context.Background())
		return usersLoadedMsg{users: users, err: err}
	}
}

func (m userManagementModel) addCmd(username string) tea.Cmd {
	return func() tea.Msg {
		if m.service == nil {
			return userOpDoneMsg{err: fmt.Errorf("user service unavailable")}
		}
		_, err := m.service.Add(context.Background(), userops.AddRequest{Username: username})
		return userOpDoneMsg{status: "user added: " + username, err: err}
	}
}

func (m userManagementModel) toggleCmd(user state.User) tea.Cmd {
	return func() tea.Msg {
		if m.service == nil {
			return userOpDoneMsg{err: fmt.Errorf("user service unavailable")}
		}
		if user.Status == state.UserStatusPaused {
			err := m.service.Activate(context.Background(), user.Username)
			return userOpDoneMsg{status: "user activated: " + user.Username, err: err}
		}
		err := m.service.Deactivate(context.Background(), user.Username)
		return userOpDoneMsg{status: "user deactivated: " + user.Username, err: err}
	}
}

func (m userManagementModel) removeCmd(user state.User) tea.Cmd {
	return func() tea.Msg {
		if m.service == nil {
			return userOpDoneMsg{err: fmt.Errorf("user service unavailable")}
		}
		if user.Status == state.UserStatusActive {
			if err := m.service.Deactivate(context.Background(), user.Username); err != nil {
				return userOpDoneMsg{err: err}
			}
		}
		err := m.service.Remove(context.Background(), userops.RemoveRequest{Username: user.Username})
		return userOpDoneMsg{status: "user removed: " + user.Username, err: err}
	}
}

func (m userManagementModel) backupCmd(user state.User) tea.Cmd {
	return func() tea.Msg {
		if m.backups == nil {
			return userOpDoneMsg{err: fmt.Errorf("backup service unavailable")}
		}
		backup, err := m.backups.Create(context.Background(), backupops.CreateRequest{Username: user.Username})
		status := "backup created"
		if backup != nil {
			status = "backup created: " + backup.ID
		}
		return userOpDoneMsg{status: status, err: err}
	}
}

func (m userManagementModel) loadBackupsCmd(user state.User) tea.Cmd {
	return func() tea.Msg {
		if m.backups == nil {
			return backupsLoadedMsg{err: fmt.Errorf("backup service unavailable")}
		}
		backups, err := m.backups.List(context.Background(), user.Username)
		return backupsLoadedMsg{backups: backups, err: err}
	}
}

func (m userManagementModel) restoreCmd(user state.User, backup state.Backup) tea.Cmd {
	return func() tea.Msg {
		if m.backups == nil {
			return userOpDoneMsg{err: fmt.Errorf("backup service unavailable")}
		}
		err := m.backups.Restore(context.Background(), backupops.RestoreRequest{Username: user.Username, BackupID: backup.ID})
		return userOpDoneMsg{status: "backup restored: " + backup.ID, err: err}
	}
}

func printableKey(msg tea.KeyMsg) string {
	s := msg.String()
	if len(s) == 1 {
		return s
	}
	return ""
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
