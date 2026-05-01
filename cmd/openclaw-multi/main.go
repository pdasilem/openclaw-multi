package main

import (
	"context"
	"fmt"
	"os"

	"github.com/pdasilem/openclaw-multi/internal/audit"
	"github.com/pdasilem/openclaw-multi/internal/backup"
	"github.com/pdasilem/openclaw-multi/internal/shell"
	"github.com/pdasilem/openclaw-multi/internal/state"
	"github.com/pdasilem/openclaw-multi/internal/systemprep"
	"github.com/pdasilem/openclaw-multi/internal/tui"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "system-prepare" {
		if err := runSystemPrepare(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "fatal:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "backup" {
		if err := runBackup(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "fatal:", err)
			os.Exit(1)
		}
		return
	}
	if err := tui.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func runSystemPrepare(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: sudo openclaw-multi system-prepare")
	}
	ctx := context.Background()
	exec := &shell.RealExecutor{Actor: "system-prepare"}
	actions, err := systemprep.Run(ctx, exec)
	if err != nil {
		return err
	}
	for _, action := range actions {
		fmt.Println(action)
	}
	fmt.Println("system preparation complete")
	return nil
}

func runBackup(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: openclaw-multi backup <username>")
	}
	ctx := context.Background()
	statePath := ""
	if stateDir := os.Getenv("OPENCLAW_STATE_DIR"); stateDir != "" {
		statePath = stateDir + "/state.db"
	}
	store, err := state.Open(ctx, statePath)
	if err != nil {
		return fmt.Errorf("open state: %w", err)
	}
	defer func() { _ = store.Close() }()

	logger, err := audit.New(os.Getenv("OPENCLAW_AUDIT_LOG"))
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer func() { _ = logger.Close() }()

	exec := &shell.RealExecutor{Logger: logger}
	manager := backup.NewManager(store, exec, shell.RealFS{}, logger, backup.Options{TemplateDir: "/opt/openclaw-multi/templates"})
	b, err := manager.Create(ctx, backup.CreateRequest{Username: args[0]})
	if err != nil {
		return err
	}
	fmt.Println(b.Path)
	return nil
}
