// Package audit provides JSONL audit log writing per master plan §12 OQ-1.
package audit

// ActionType is the type of action being audited.
type ActionType string

// Action type constants for audit log entries.
const (
	ActionBootstrapUser  ActionType = "bootstrap_user"
	ActionDeleteUser     ActionType = "delete_user"
	ActionUpdateOpenclaw ActionType = "update_openclaw"
	ActionAddRoute       ActionType = "add_route"
	ActionDeleteRoute    ActionType = "delete_route"
	ActionEnableUser     ActionType = "enable_user"
	ActionDisableUser    ActionType = "disable_user"
	ActionBackupCreate   ActionType = "backup_create"
	ActionBackupRestore  ActionType = "backup_restore"
	ActionDoctorRun      ActionType = "doctor_run"
	ActionDoctorFix      ActionType = "doctor_fix"
	ActionUninstall      ActionType = "uninstall"
	ActionAdminSet       ActionType = "admin_set"
	ActionStartup        ActionType = "startup"
	ActionShutdown       ActionType = "shutdown"
	ActionShellExec      ActionType = "shell_exec"
)

// Result is the outcome of an audited action.
type Result string

// Result constants for audit log entries.
const (
	ResultOk         Result = "ok"
	ResultError      Result = "error"
	ResultRolledBack Result = "rolled_back"
)
