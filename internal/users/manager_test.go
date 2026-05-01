package users

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/pdasilem/openclaw-multi/internal/audit"
	"github.com/pdasilem/openclaw-multi/internal/config"
	"github.com/pdasilem/openclaw-multi/internal/shell"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

type auditRecorder struct {
	events []audit.Event
}

type failingRoutes struct {
	err error
}

type removeHook struct {
	called string
	err    error
}

func (h *removeHook) BeforeRemove(_ context.Context, username string) error {
	h.called = username
	return h.err
}

func (f failingRoutes) EnableUserGateway(context.Context, state.User) (state.Route, error) {
	return state.Route{}, f.err
}

func (f failingRoutes) DisableUserRoutes(context.Context, string) error { return f.err }
func (f failingRoutes) DeleteUserRoutes(context.Context, string) error  { return f.err }

func (r *auditRecorder) Emit(e audit.Event) error {
	r.events = append(r.events, e)
	return nil
}

func TestManagerAddSuccess(t *testing.T) {
	ctx := context.Background()
	store := openUserTestStore(t)
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{
			shell.OKResponse(""),
			shell.OKResponse(""),
			shell.OKResponse("1001\n"),
			shell.OKResponse(""),
		},
	}
	fs := watcherFS()
	log := &auditRecorder{}
	m := testManager(store, exec, fs, log)

	user, err := m.Add(ctx, AddRequest{Username: "alice"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if user.Username != "alice" || user.Port != 18789 || user.UID != 1001 {
		t.Fatalf("unexpected user: %+v", user)
	}
	if user.GatewayURL != "https://gateway-alice.ui.example.com" {
		t.Fatalf("GatewayURL: got %q", user.GatewayURL)
	}
	if exec.CallCount() != 12 {
		t.Fatalf("expected 12 command calls, got %d", exec.CallCount())
	}
	if !envContains(exec.Calls[5].Env, "OPENCLAW_GATEWAY_TOKEN=") {
		t.Fatalf("expected gateway token env in onboard call: %+v", exec.Calls[5].Env)
	}
	if !strings.Contains(strings.Join(exec.Calls[5].Cmd, " "), "/home/alice/.local/bin/openclaw onboard --install-daemon") {
		t.Fatalf("expected tenant openclaw wrapper in onboard call: %+v", exec.Calls[5].Cmd)
	}
	if _, ok := fs.Files["/home/alice/.local/bin/openclaw"]; !ok {
		t.Fatal("expected tenant openclaw wrapper written")
	}
	if _, ok := fs.Files["/home/alice/.local/bin/openclaw-gateway-start"]; !ok {
		t.Fatal("expected tenant gateway wrapper written")
	}
	if data := string(fs.Files["/home/alice/.config/systemd/user/openclaw-gateway.service"]); !strings.Contains(data, "/home/alice/.local/bin/openclaw-gateway-start") {
		t.Fatalf("expected gateway unit to use tenant wrapper, got %q", data)
	}
	if _, ok := fs.Files["/home/alice/.config/systemd/user/openclaw-overlay-watcher.service"]; !ok {
		t.Fatal("expected watcher unit written")
	}
	if len(log.events) == 0 || log.events[len(log.events)-1].Result != audit.ResultOk {
		t.Fatalf("expected ok audit event, got %+v", log.events)
	}
}

func TestManagerAddBlocksMissingDomainBeforeCommands(t *testing.T) {
	store := openUserTestStore(t)
	exec := &shell.MockExecutor{}
	cfg := config.Defaults()
	cfg.Domain = ""
	m := NewManager(store, exec, watcherFS(), cfg, nil, nil)

	_, err := m.Add(context.Background(), AddRequest{Username: "alice"})
	if !errors.Is(err, ErrMissingDomain) {
		t.Fatalf("expected ErrMissingDomain, got %v", err)
	}
	if exec.CallCount() != 0 {
		t.Fatalf("expected no commands, got %d", exec.CallCount())
	}
}

func TestManagerList(t *testing.T) {
	ctx := context.Background()
	store := openUserTestStore(t)
	if err := store.UpsertUser(ctx, state.User{Username: "alice", Port: 18789, Status: state.UserStatusActive}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	m := testManager(store, &shell.MockExecutor{}, watcherFS(), nil)
	users, err := m.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(users) != 1 || users[0].Username != "alice" {
		t.Fatalf("unexpected users: %+v", users)
	}
}

func TestManagerAddValidationAndDuplicateErrorsBeforeCommands(t *testing.T) {
	ctx := context.Background()
	store := openUserTestStore(t)
	exec := &shell.MockExecutor{}
	m := testManager(store, exec, watcherFS(), nil)
	if _, err := m.Add(ctx, AddRequest{Username: "Root"}); !errors.Is(err, ErrInvalidUsername) {
		t.Fatalf("expected ErrInvalidUsername, got %v", err)
	}
	if exec.CallCount() != 0 {
		t.Fatalf("expected no commands for invalid username")
	}
	if err := store.UpsertUser(ctx, state.User{Username: "alice", Port: 18789, Status: state.UserStatusActive}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if _, err := m.Add(ctx, AddRequest{Username: "alice"}); err == nil {
		t.Fatal("expected duplicate error")
	}
	if exec.CallCount() != 0 {
		t.Fatalf("expected no commands for duplicate user")
	}
}

func TestManagerAddRejectsInvalidPortConfigBeforeCommands(t *testing.T) {
	store := openUserTestStore(t)
	exec := &shell.MockExecutor{}
	cfg := configWithDomain()
	cfg.PortRangeStep = 1
	m := NewManager(store, exec, watcherFS(), cfg, nil, nil)

	_, err := m.Add(context.Background(), AddRequest{Username: "alice"})
	if !errors.Is(err, ErrInvalidPortConfig) {
		t.Fatalf("expected ErrInvalidPortConfig, got %v", err)
	}
	if exec.CallCount() != 0 {
		t.Fatalf("expected no commands, got %d", exec.CallCount())
	}
}

func TestManagerAddCommandAndParseFailures(t *testing.T) {
	tests := []struct {
		name string
		exec *shell.MockExecutor
	}{
		{name: "useradd", exec: shell.FailAt(0, "useradd failed")},
		{name: "uid parse", exec: &shell.MockExecutor{Responses: []shell.ExecResult{
			shell.OKResponse(""),
			shell.OKResponse(""),
			shell.OKResponse("not-a-uid\n"),
		}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testManager(openUserTestStore(t), tt.exec, watcherFS(), nil)
			_, err := m.Add(context.Background(), AddRequest{Username: "alice"})
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestManagerAddRunsSystemCommandsWithSudo(t *testing.T) {
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{
		shell.OKResponse(""),
		shell.OKResponse(""),
		shell.OKResponse("1001\n"),
		shell.OKResponse(""),
	}}
	m := testManager(openUserTestStore(t), exec, watcherFS(), nil)
	_, _ = m.Add(context.Background(), AddRequest{Username: "alice"})
	for i, call := range exec.Calls[:2] {
		if !call.Sudo {
			t.Fatalf("expected setup command %d to use sudo: %v", i, call.Cmd)
		}
	}
}

func TestManagerAddWatcherTemplateFailure(t *testing.T) {
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{
		shell.OKResponse(""),
		shell.OKResponse(""),
		shell.OKResponse("1001\n"),
		shell.OKResponse(""),
	}}
	fs := shell.NewMemFS()
	fs.Files["templates/openclaw-gateway.service.tmpl"] = []byte("ExecStart=/home/${USERNAME}/.local/bin/openclaw-gateway-start\n")
	m := testManager(openUserTestStore(t), exec, fs, nil)
	_, err := m.Add(context.Background(), AddRequest{Username: "alice"})
	if err == nil || !strings.Contains(err.Error(), "read watcher template") {
		t.Fatalf("expected watcher template error, got %v", err)
	}
}

func TestManagerAddWatcherTemplateRenderFailure(t *testing.T) {
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{
		shell.OKResponse(""),
		shell.OKResponse(""),
		shell.OKResponse("1001\n"),
		shell.OKResponse(""),
	}}
	fs := shell.NewMemFS()
	fs.Files["templates/openclaw-gateway.service.tmpl"] = []byte("ExecStart=/home/${USERNAME}/.local/bin/openclaw-gateway-start\n")
	fs.Files["templates/openclaw-overlay-watcher.service.tmpl"] = []byte("${UNKNOWN}\n")
	m := testManager(openUserTestStore(t), exec, fs, nil)
	_, err := m.Add(context.Background(), AddRequest{Username: "alice"})
	if err == nil || !strings.Contains(err.Error(), "render watcher template") {
		t.Fatalf("expected render watcher template error, got %v", err)
	}
}

func TestManagerAddRouteFailureRollsBackState(t *testing.T) {
	ctx := context.Background()
	store := openUserTestStore(t)
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{
		shell.OKResponse(""),
		shell.OKResponse(""),
		shell.OKResponse("1001\n"),
		shell.OKResponse(""),
	}}
	m := testManager(store, exec, watcherFS(), nil)
	m.Routes = failingRoutes{err: fmt.Errorf("route failed")}

	_, err := m.Add(ctx, AddRequest{Username: "alice"})
	if err == nil {
		t.Fatal("expected route failure")
	}
	_, err = store.GetUser(ctx, "alice")
	if !errors.Is(err, state.ErrNoUser) {
		t.Fatalf("expected rollback delete, got %v", err)
	}
}

func TestManagerAddLateCommandFailures(t *testing.T) {
	for _, failAt := range []int{3, 4, 5, 6, 7, 8, 9, 10, 11} {
		t.Run(fmt.Sprintf("fail-at-%d", failAt), func(t *testing.T) {
			responses := []shell.ExecResult{
				shell.OKResponse(""),
				shell.OKResponse(""),
				shell.OKResponse("1001\n"),
				shell.OKResponse(""),
				shell.OKResponse(""),
				shell.OKResponse(""),
				shell.OKResponse(""),
				shell.OKResponse(""),
				shell.OKResponse(""),
				shell.OKResponse(""),
				shell.OKResponse(""),
				shell.OKResponse(""),
			}
			errs := make([]error, len(responses))
			responses[failAt] = shell.ExecResult{Stderr: "boom", ExitCode: 1}
			errs[failAt] = fmt.Errorf("boom at %d", failAt)
			m := testManager(openUserTestStore(t), &shell.MockExecutor{Responses: responses, Errors: errs}, watcherFS(), nil)
			_, err := m.Add(context.Background(), AddRequest{Username: "alice"})
			if err == nil {
				t.Fatal("expected command failure")
			}
		})
	}
}

func TestManagerDeactivateActivateAndRemove(t *testing.T) {
	ctx := context.Background()
	store := openUserTestStore(t)
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{shell.OKResponse("")}}
	m := testManager(store, exec, watcherFS(), nil)
	user := state.User{
		Username:   "alice",
		UID:        1001,
		Port:       18789,
		Status:     state.UserStatusActive,
		Linger:     true,
		GatewayURL: "https://gateway-alice.ui.example.com",
	}
	if err := store.UpsertUser(ctx, user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if _, err := m.Routes.EnableUserGateway(ctx, user); err != nil {
		t.Fatalf("EnableUserGateway: %v", err)
	}

	if err := m.Deactivate(ctx, "alice"); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	got, err := store.GetUser(ctx, "alice")
	if err != nil {
		t.Fatalf("GetUser paused: %v", err)
	}
	if got.Status != state.UserStatusPaused {
		t.Fatalf("expected paused, got %q", got.Status)
	}

	if err := m.Activate(ctx, "alice"); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	got, err = store.GetUser(ctx, "alice")
	if err != nil {
		t.Fatalf("GetUser active: %v", err)
	}
	if got.Status != state.UserStatusActive {
		t.Fatalf("expected active, got %q", got.Status)
	}

	if err := m.Remove(ctx, RemoveRequest{Username: "alice"}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	_, err = store.GetUser(ctx, "alice")
	if !errors.Is(err, state.ErrNoUser) {
		t.Fatalf("expected ErrNoUser after remove, got %v", err)
	}
	port, err := AllocateGatewayPort(configWithDomain(), nil)
	if err != nil {
		t.Fatalf("AllocateGatewayPort: %v", err)
	}
	if port != 18789 {
		t.Fatalf("expected port freed for reuse, got %d", port)
	}
}

func TestManagerDeactivateActivateIdempotentAndUnknown(t *testing.T) {
	ctx := context.Background()
	store := openUserTestStore(t)
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{shell.OKResponse("")}}
	m := testManager(store, exec, watcherFS(), nil)

	if err := m.Deactivate(ctx, "missing"); !errors.Is(err, state.ErrNoUser) {
		t.Fatalf("expected ErrNoUser, got %v", err)
	}
	if err := store.UpsertUser(ctx, state.User{Username: "alice", Port: 18789, Status: state.UserStatusPaused}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	exec.Reset()
	if err := m.Deactivate(ctx, "alice"); err != nil {
		t.Fatalf("Deactivate paused: %v", err)
	}
	if exec.CallCount() != 0 {
		t.Fatalf("expected no commands for paused deactivate, got %d", exec.CallCount())
	}
	if err := m.Activate(ctx, "alice"); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	exec.Reset()
	if err := m.Activate(ctx, "alice"); err != nil {
		t.Fatalf("Activate active: %v", err)
	}
	if exec.CallCount() != 0 {
		t.Fatalf("expected no commands for active activate, got %d", exec.CallCount())
	}
}

func TestManagerDeactivateFailures(t *testing.T) {
	tests := []struct {
		name   string
		exec   shell.Executor
		routes RoutePublisher
	}{
		{name: "stop services", exec: shell.FailAt(0, "stop failed")},
		{name: "disable linger", exec: shell.FailAt(1, "linger failed")},
		{name: "route disable", exec: &shell.MockExecutor{Responses: []shell.ExecResult{shell.OKResponse("")}}, routes: failingRoutes{err: fmt.Errorf("route disable failed")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			store := openUserTestStore(t)
			m := testManager(store, tt.exec, watcherFS(), nil)
			if tt.routes != nil {
				m.Routes = tt.routes
			}
			if err := store.UpsertUser(ctx, state.User{Username: "alice", Port: 18789, Status: state.UserStatusActive}); err != nil {
				t.Fatalf("UpsertUser: %v", err)
			}
			if err := m.Deactivate(ctx, "alice"); err == nil {
				t.Fatal("expected deactivate error")
			}
		})
	}
}

func TestManagerActivateFailures(t *testing.T) {
	tests := []struct {
		name   string
		exec   shell.Executor
		routes RoutePublisher
	}{
		{name: "route enable", exec: &shell.MockExecutor{}, routes: failingRoutes{err: fmt.Errorf("route enable failed")}},
		{name: "enable linger", exec: shell.FailAt(0, "linger failed")},
		{name: "start services", exec: shell.FailAt(1, "start failed")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			store := openUserTestStore(t)
			m := testManager(store, tt.exec, watcherFS(), nil)
			if tt.routes != nil {
				m.Routes = tt.routes
			}
			if err := store.UpsertUser(ctx, state.User{Username: "alice", Port: 18789, Status: state.UserStatusPaused}); err != nil {
				t.Fatalf("UpsertUser: %v", err)
			}
			if err := m.Activate(ctx, "alice"); err == nil {
				t.Fatal("expected activate error")
			}
		})
	}
}

func TestManagerRemovePausedAndUnknown(t *testing.T) {
	ctx := context.Background()
	store := openUserTestStore(t)
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{shell.OKResponse("")}}
	m := testManager(store, exec, watcherFS(), nil)

	if err := m.Remove(ctx, RemoveRequest{Username: "missing"}); !errors.Is(err, state.ErrNoUser) {
		t.Fatalf("expected ErrNoUser, got %v", err)
	}
	if err := store.UpsertUser(ctx, state.User{Username: "alice", Port: 18789, Status: state.UserStatusPaused}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if err := m.Remove(ctx, RemoveRequest{Username: "alice"}); err != nil {
		t.Fatalf("Remove paused: %v", err)
	}
	if exec.CallCount() != 3 {
		t.Fatalf("expected 3 remove commands for paused user, got %d", exec.CallCount())
	}
}

func TestManagerRemoveRunsBackupHookBeforeDestructiveCommands(t *testing.T) {
	ctx := context.Background()
	store := openUserTestStore(t)
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{shell.OKResponse("")}}
	m := testManager(store, exec, watcherFS(), nil)
	hook := &removeHook{}
	m.BeforeRemove = hook
	if err := store.UpsertUser(ctx, state.User{Username: "alice", Port: 18789, Status: state.UserStatusPaused}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if err := m.Remove(ctx, RemoveRequest{Username: "alice"}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if hook.called != "alice" {
		t.Fatalf("expected backup hook for alice, got %q", hook.called)
	}
	if exec.CallCount() != 3 {
		t.Fatalf("expected destructive commands after backup hook, got %d", exec.CallCount())
	}
}

func TestManagerRemoveAbortsWhenBackupHookFails(t *testing.T) {
	ctx := context.Background()
	store := openUserTestStore(t)
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{shell.OKResponse("")}}
	m := testManager(store, exec, watcherFS(), nil)
	m.BeforeRemove = &removeHook{err: fmt.Errorf("backup failed")}
	if err := store.UpsertUser(ctx, state.User{Username: "alice", Port: 18789, Status: state.UserStatusPaused}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if err := m.Remove(ctx, RemoveRequest{Username: "alice"}); err == nil {
		t.Fatal("expected backup hook error")
	}
	if exec.CallCount() != 0 {
		t.Fatalf("expected no destructive commands after backup failure, got %d", exec.CallCount())
	}
}

func TestManagerRemoveFailures(t *testing.T) {
	tests := []struct {
		name   string
		exec   shell.Executor
		routes RoutePublisher
	}{
		{name: "route delete", exec: &shell.MockExecutor{}, routes: failingRoutes{err: fmt.Errorf("route delete failed")}},
		{name: "uninstall", exec: shell.FailAt(0, "uninstall failed")},
		{name: "disable linger", exec: shell.FailAt(1, "linger failed")},
		{name: "userdel", exec: shell.FailAt(2, "userdel failed")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			store := openUserTestStore(t)
			m := testManager(store, tt.exec, watcherFS(), nil)
			if tt.routes != nil {
				m.Routes = tt.routes
			}
			if err := store.UpsertUser(ctx, state.User{Username: "alice", Port: 18789, Status: state.UserStatusPaused}); err != nil {
				t.Fatalf("UpsertUser: %v", err)
			}
			if err := m.Remove(ctx, RemoveRequest{Username: "alice"}); err == nil {
				t.Fatal("expected remove error")
			}
		})
	}
}

func TestManagerReadyErrors(t *testing.T) {
	m := &Manager{}
	if err := m.ready(); err == nil {
		t.Fatal("expected missing store error")
	}
	m.Store = openUserTestStore(t)
	if err := m.ready(); err == nil {
		t.Fatal("expected missing executor error")
	}
	m.Exec = &shell.MockExecutor{}
	if err := m.ready(); err == nil {
		t.Fatal("expected missing filesystem error")
	}
	m.FS = watcherFS()
	if err := m.ready(); err == nil {
		t.Fatal("expected missing route publisher error")
	}
	m.Routes = StateRoutePublisher{Store: m.Store, Config: configWithDomain()}
	if err := m.ready(); err != nil {
		t.Fatalf("expected ready success, got %v", err)
	}
}

func testManager(store *state.Store, exec shell.Executor, fs *shell.MemFS, log *auditRecorder) *Manager {
	cfg := configWithDomain()
	var logger Auditor
	if log != nil {
		logger = log
	}
	m := NewManager(store, exec, fs, cfg, nil, logger)
	m.TemplateDir = "templates"
	return m
}

func configWithDomain() *config.OverlayConfig {
	cfg := config.Defaults()
	cfg.Domain = "example.com"
	cfg.Subdomain = "ui"
	return cfg
}

func watcherFS() *shell.MemFS {
	fs := shell.NewMemFS()
	fs.Files["templates/openclaw-overlay-watcher.service.tmpl"] = []byte("user=${USERNAME}\napi=${OVERLAY_API_ENDPOINT}\n")
	fs.Files["templates/openclaw-gateway.service.tmpl"] = []byte("ExecStart=/home/${USERNAME}/.local/bin/openclaw-gateway-start\nport=${GATEWAY_PORT}\ntoken=${GATEWAY_TOKEN}\n")
	return fs
}

func envContains(env []string, prefix string) bool {
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return true
		}
	}
	return false
}
