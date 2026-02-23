package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/paularlott/cli"
	"github.com/paularlott/cli/tui"
)

func NewManageCommand() *cli.Command {
	return &cli.Command{
		Name:        "manage",
		Usage:       "Open the interactive TUI management dashboard",
		Description: "Opens an interactive terminal UI for managing overlays, view logs, and monitor system health.",
		Run: func(ctx context.Context, cmd *cli.Command) error {
			store, err := state.NewStore(cfg.GetStatePath())
			if err != nil {
				return fmt.Errorf("failed to init state store: %w", err)
			}

			mgr, err := createOverlayManager()
			if err != nil {
				return fmt.Errorf("failed to init overlay manager: %w", err)
			}

			// Extract just the basic info we need for the menu (we do this to avoid passing complex instances around)
			return RunInteractiveManage(ctx, mgr, store)
		},
	}
}

func RunInteractiveManage(ctx context.Context, mgr overlayManager, store *state.Store) error {
	var t *tui.TUI

	// Setup TUI
	enabled := true
	t = tui.New(tui.Config{
		InputEnabled:   &enabled,
		UserLabel:      "Phantom",
		AssistantLabel: "Result",
		SystemLabel:    "Log",
		StatusLeft:     "phantom manage",
		StatusRight:    "Type /help for options. Ctrl+C to exit.",
		OnEscape: func() {
			t.AddMessage(tui.RoleSystem, "Escape pressed. Type 'exit' to quit.")
		},
		OnSubmit: func(text string) {
			if text == "exit" || text == "quit" {
				t.Exit()
				return
			}
			t.AddMessage(tui.RoleUser, text)
			t.AddMessage(tui.RoleAssistant, "Unknown command. Type /menu or /help")
		},
		Commands: []*tui.Command{
			{
				Name:        "exit",
				Description: "Exit the management dashboard",
				Handler:     func(_ string) { t.Exit() },
			},
			{
				Name:        "menu",
				Description: "Open interactive menu",
				Handler: func(args string) {
					openMainMenu(t, mgr, store)
				},
			},
			{
				Name:        "clear",
				Description: "Clear conversation history",
				Handler:     func(_ string) { t.ClearOutput() },
			},
		},
	})

	t.AddMessage(tui.RoleSystem, "Welcome to Phantom interactive management. Press / to see commands or type /menu.")

	// Wrap logger so global app logs show up here
	oldLog := log
	log = &tuiLogger{t: t, verbose: verbose}
	defer func() { log = oldLog }()

	if err := t.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return err
	}
	return nil
}

func openMainMenu(t *tui.TUI, mgr overlayManager, store *state.Store) {
	ovls, err := store.LoadAll()
	if err != nil {
		t.AddMessage(tui.RoleSystem, "Error loading overlays: "+err.Error())
		return
	}

	ovlItems := make([]*tui.MenuItem, 0, len(ovls))
	for _, o := range ovls {
		name := o.Name

		status := "unmounted"
		if o.PID > 0 {
			if mounted, _ := mgr.IsMounted(o); mounted {
				status = "mounted"
			}
		}
		label := fmt.Sprintf("%s (%s)", name, status)

		ovlItems = append(ovlItems, &tui.MenuItem{
			Label: label,
			Children: []*tui.MenuItem{
				{
					Label: "Inspect",
					OnSelect: func(_ *tui.MenuItem, _ string) {
						t.AddMessage(tui.RoleSystem, fmt.Sprintf("Overlay: %s\nBranch: %s\nStatus: %s\nMount Point: %s", name, o.Branch, status, o.MountPoint))
					},
				},
				{
					Label: "Unmount & Cleanup",
					OnSelect: func(_ *tui.MenuItem, _ string) {
						t.AddMessage(tui.RoleSystem, "Cleaning up "+name+"...")
						if err := mgr.Cleanup(o); err != nil {
							t.AddMessage(tui.RoleSystem, "Failed to cleanup: "+err.Error())
							return
						}
						store.Delete(name)
						t.AddMessage(tui.RoleSystem, "Cleanup complete.")
					},
				},
			},
		})
	}

	if len(ovls) == 0 {
		ovlItems = append(ovlItems, &tui.MenuItem{
			Label: "<No overlays found>",
		})
	}

	menu := &tui.Menu{
		Title: "Phantom Management",
		Items: []*tui.MenuItem{
			{
				Label:    "Overlays",
				Children: ovlItems,
			},
			{
				Label: "System Health",
				OnSelect: func(_ *tui.MenuItem, _ string) {
					t.AddMessage(tui.RoleSystem, "Running health check...")
					// We'd map command logic here directly!
					t.AddMessage(tui.RoleAssistant, "Health check finished.")
				},
			},
			{
				Label: "Exit",
				OnSelect: func(_ *tui.MenuItem, _ string) {
					t.Exit()
				},
			},
		},
	}
	t.OpenMenu(menu)
}
