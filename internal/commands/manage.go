package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/martinsuchenak/phantom/internal/git"
	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/martinsuchenak/phantom/pkg/api"
	"github.com/paularlott/cli"
	"github.com/paularlott/cli/tui"
)

func NewManageCommand() *cli.Command {
	return &cli.Command{
		Name:        "manage",
		Usage:       "Open the interactive TUI management dashboard",
		Description: "Opens an interactive terminal UI for managing overlays, view logs, and monitor system health.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:         "theme",
				Usage:        "TUI color theme (default, amber, blue, green, purple, light, plain)",
				DefaultValue: "default",
			},
		},
		Run: func(ctx context.Context, cmd *cli.Command) error {
			store, err := state.NewStore(cfg.GetStatePath())
			if err != nil {
				return fmt.Errorf("failed to init state store: %w", err)
			}

			mgr, err := createOverlayManager()
			if err != nil {
				return fmt.Errorf("failed to init overlay manager: %w", err)
			}

			themeName := cmd.GetString("theme")
			theme, ok := tui.ThemeByName(themeName)
			if !ok {
				return fmt.Errorf("unknown theme %q — valid themes: %s", themeName, strings.Join(tui.ThemeNames(), ", "))
			}

			return RunInteractiveManage(ctx, mgr, store, theme)
		},
	}
}

// ─── TUI entry point ──────────────────────────────────────────────────────────

func RunInteractiveManage(ctx context.Context, mgr overlayManager, store *state.Store, theme *tui.Theme) error {
	var t *tui.TUI

	enabled := true
	t = tui.New(tui.Config{
		Theme:          theme,
		InputEnabled:   &enabled,
		UserLabel:      "CMD",
		AssistantLabel: "Result",
		SystemLabel:    "Info",
		StatusLeft:     "phantom manage",
		StatusRight:    "/menu • /start • /run • /run-all • /run-chain • /health • /prune • /gc • /exit",
		OnEscape:       func() { t.AddMessage(tui.RoleSystem, "Type /menu to open the dashboard, /exit to quit.") },
		OnSubmit: func(text string) {
			text = strings.TrimSpace(text)
			switch text {
			case "exit", "quit":
				t.Exit()
			default:
				t.AddMessage(tui.RoleUser, text)
				t.AddMessage(tui.RoleAssistant, "Unknown input. Use /menu or /help to see available commands.")
			}
		},
		Commands: []*tui.Command{
			{Name: "exit", Description: "Exit the management dashboard", Handler: func(_ string) { t.Exit() }},
			{Name: "menu", Description: "Open the interactive management menu", Handler: func(_ string) { openMainMenu(ctx, t, mgr, store) }},
			{
				Name:        "start",
				Description: "Start a new overlay: /start <base-dir> [name] [branch]",
				Handler: func(args string) {
					parts := strings.Fields(args)
					if len(parts) == 0 {
						t.AddMessage(tui.RoleSystem, "Usage: /start <base-dir> [name] [branch]")
						return
					}
					baseDir := parts[0]
					name := ""
					if len(parts) > 1 {
						name = parts[1]
					}
					branch := ""
					if len(parts) > 2 {
						branch = parts[2]
					}
					runTUIStart(ctx, t, baseDir, name, branch, false)
				},
			},
			{
				Name:        "run",
				Description: "Run a single agent: /run <base-dir> <agent-cmd> [task]",
				Handler: func(args string) {
					parts := strings.Fields(strings.TrimSpace(args))
					if len(parts) < 2 {
						t.AddMessage(tui.RoleSystem, "Usage: /run <base-dir> <agent-cmd> [task]")
						return
					}
					task := ""
					if len(parts) > 2 {
						task = strings.Join(parts[2:], " ")
					}
					go runTUIRun(ctx, t, parts[0], parts[1], task, "", "", "", 0, false, false, false)
				},
			},
			{
				Name:        "run-all",
				Description: "Run agents in parallel: /run-all <base-dir> <config.yaml>",
				Handler: func(args string) {
					parts := strings.Fields(strings.TrimSpace(args))
					if len(parts) < 2 {
						t.AddMessage(tui.RoleSystem, "Usage: /run-all <base-dir> <config.yaml>")
						return
					}
					go runTUIRunAll(ctx, t, parts[0], parts[1])
				},
			},
			{
				Name:        "run-chain",
				Description: "Run agents sequentially: /run-chain <base-dir> <config.yaml>",
				Handler: func(args string) {
					parts := strings.Fields(strings.TrimSpace(args))
					if len(parts) < 2 {
						t.AddMessage(tui.RoleSystem, "Usage: /run-chain <base-dir> <config.yaml>")
						return
					}
					go runTUIRunChain(ctx, t, parts[0], parts[1])
				},
			},
			{Name: "health", Description: "Run a system health check", Handler: func(_ string) { runHealthInner(t, mgr, store, false) }},
			{Name: "health-fix", Description: "Run health check and auto-fix issues", Handler: func(_ string) { runHealthInner(t, mgr, store, true) }},
			{Name: "prune", Description: "Prune unmounted/stale overlays", Handler: func(_ string) { runTUIPrune(t, mgr, store, false) }},
			{Name: "prune-dry", Description: "Dry-run prune (preview only)", Handler: func(_ string) { runTUIPrune(t, mgr, store, true) }},
			{Name: "gc", Description: "Garbage-collect orphaned resources", Handler: func(_ string) { runTUIGC(ctx, t, store, false) }},
			{Name: "gc-dry", Description: "Dry-run GC (preview only)", Handler: func(_ string) { runTUIGC(ctx, t, store, true) }},
			{
				Name:        "theme",
				Description: "Switch color theme: /theme <name>  (default, amber, blue, green, purple, light, plain)",
				Handler: func(args string) {
					args = strings.TrimSpace(args)
					if args == "" {
						t.AddMessage(tui.RoleSystem, "Usage: /theme <name>\nAvailable: "+strings.Join(tui.ThemeNames(), ", "))
						return
					}
					th, ok := tui.ThemeByName(args)
					if !ok {
						t.AddMessage(tui.RoleSystem, "Unknown theme "+args+". Available: "+strings.Join(tui.ThemeNames(), ", "))
						return
					}
					t.SetTheme(th)
					t.AddMessage(tui.RoleSystem, "Theme switched to "+args+".")
				},
			},
			{Name: "clear", Description: "Clear the output", Handler: func(_ string) { t.ClearOutput() }},
		},
	})

	t.AddMessage(tui.RoleSystem,
		"Welcome to Phantom Management Dashboard.\n"+
			"Use ↑/↓ and Enter to navigate the menu, Esc to go back.\n"+
			"Type /help for slash commands or /exit to quit.",
	)

	// Auto-open the menu immediately
	openMainMenu(ctx, t, mgr, store)

	return t.Run(ctx)
}

// ─── Main menu ────────────────────────────────────────────────────────────────

func openMainMenu(ctx context.Context, t *tui.TUI, mgr overlayManager, store *state.Store) {
	ovls, err := store.LoadAll()
	if err != nil {
		t.AddMessage(tui.RoleSystem, "Error loading overlays: "+err.Error())
		return
	}

	// Build per-overlay sub-items
	ovlItems := buildOverlayItemsTyped(ctx, t, mgr, store, ovls)

	// Build theme-switcher items
	themeItems := make([]*tui.MenuItem, 0, len(tui.ThemeNames()))
	for _, name := range tui.ThemeNames() {
		name := name
		themeItems = append(themeItems, &tui.MenuItem{
			Label: name,
			OnSelect: func(_ *tui.MenuItem, _ string) {
				if th, ok := tui.ThemeByName(name); ok {
					t.SetTheme(th)
					t.AddMessage(tui.RoleSystem, "Theme switched to "+name+".")
				}
			},
		})
	}

	menu := &tui.Menu{
		Title: "Phantom Management",
		Items: []*tui.MenuItem{
			{
				Label: "Start New Overlay",
				Children: []*tui.MenuItem{
					{
						Label:  "Quick start  (auto-name, auto-branch)",
						Prompt: "Base directory path:",
						OnSelect: func(_ *tui.MenuItem, input string) {
							input = strings.TrimSpace(input)
							if input == "" {
								t.AddMessage(tui.RoleSystem, "Cancelled.")
								return
							}
							runTUIStart(ctx, t, input, "", "", false)
						},
					},
					{
						Label:  "Custom  (base-dir  [name]  [branch])",
						Prompt: "Enter: base-dir  [name]  [branch]",
						OnSelect: func(_ *tui.MenuItem, input string) {
							parts := strings.Fields(strings.TrimSpace(input))
							if len(parts) == 0 {
								t.AddMessage(tui.RoleSystem, "Cancelled.")
								return
							}
							baseDir := parts[0]
							name := ""
							if len(parts) > 1 {
								name = parts[1]
							}
							branch := ""
							if len(parts) > 2 {
								branch = parts[2]
							}
							runTUIStart(ctx, t, baseDir, name, branch, false)
						},
					},
					{
						Label:  "Persistent  (base-dir  [name]  [branch])",
						Prompt: "Enter: base-dir  [name]  [branch]",
						OnSelect: func(_ *tui.MenuItem, input string) {
							parts := strings.Fields(strings.TrimSpace(input))
							if len(parts) == 0 {
								t.AddMessage(tui.RoleSystem, "Cancelled.")
								return
							}
							baseDir := parts[0]
							name := ""
							if len(parts) > 1 {
								name = parts[1]
							}
							branch := ""
							if len(parts) > 2 {
								branch = parts[2]
							}
							runTUIStart(ctx, t, baseDir, name, branch, true)
						},
					},
				},
			},
			{
				Label: "Run Agents",
				Children: []*tui.MenuItem{
					// ── Single run ─────────────────────────────────────────
					{
						Label:  "Run  (single agent)",
						Prompt: "Enter: base-dir  agent-cmd  [task]",
						OnSelect: func(_ *tui.MenuItem, input string) {
							parts := strings.Fields(strings.TrimSpace(input))
							if len(parts) < 2 {
								t.AddMessage(tui.RoleSystem, "Need at least: base-dir  agent-cmd")
								return
							}
							task := ""
							if len(parts) > 2 {
								task = strings.Join(parts[2:], " ")
							}
							go runTUIRun(ctx, t, parts[0], parts[1], task, "", "", "", 0, false, false, false)
						},
					},
					{
						Label: "Run  (guided)",
						Children: []*tui.MenuItem{
							{
								Label:  "Base directory",
								Prompt: "Base directory path:",
								OnSelect: func(_ *tui.MenuItem, baseDir string) {
									baseDir = strings.TrimSpace(baseDir)
									if baseDir == "" {
										t.AddMessage(tui.RoleSystem, "Cancelled.")
										return
									}
									// Open nested menu for this base dir
									t.OpenMenu(&tui.Menu{
										Title: "Run in " + baseDir,
										Items: []*tui.MenuItem{
											{
												Label:  "Agent command",
												Prompt: "Agent command (e.g. claude  or  cursor):",
												OnSelect: func(_ *tui.MenuItem, agentCmd string) {
													agentCmd = strings.TrimSpace(agentCmd)
													if agentCmd == "" {
														t.AddMessage(tui.RoleSystem, "Cancelled.")
														return
													}
													t.OpenMenu(&tui.Menu{
														Title: "Run options",
														Items: []*tui.MenuItem{
															{
																Label: "Run now  (no task, no cleanup)",
																OnSelect: func(_ *tui.MenuItem, _ string) {
																	go runTUIRun(ctx, t, baseDir, agentCmd, "", "", "", "", 0, false, false, false)
																},
															},
															{
																Label:  "Run with task",
																Prompt: "Task description:",
																OnSelect: func(_ *tui.MenuItem, task string) {
																	go runTUIRun(ctx, t, baseDir, agentCmd, task, "", "", "", 0, false, false, false)
																},
															},
															{
																Label:  "Run with task + cleanup after",
																Prompt: "Task description:",
																OnSelect: func(_ *tui.MenuItem, task string) {
																	go runTUIRun(ctx, t, baseDir, agentCmd, task, "", "", "", 0, true, false, false)
																},
															},
														},
													})
												},
											},
										},
									})
								},
							},
						},
					},
					// ── Run-all ───────────────────────────────────────────
					{
						Label:  "Run-All  (parallel agents from config)",
						Prompt: "Enter: base-dir  config.yaml",
						OnSelect: func(_ *tui.MenuItem, input string) {
							parts := strings.Fields(strings.TrimSpace(input))
							if len(parts) < 2 {
								t.AddMessage(tui.RoleSystem, "Need: base-dir  config.yaml")
								return
							}
							go runTUIRunAll(ctx, t, parts[0], parts[1])
						},
					},
					// ── Run-chain ─────────────────────────────────────────
					{
						Label:  "Run-Chain  (sequential steps from config)",
						Prompt: "Enter: base-dir  config.yaml",
						OnSelect: func(_ *tui.MenuItem, input string) {
							parts := strings.Fields(strings.TrimSpace(input))
							if len(parts) < 2 {
								t.AddMessage(tui.RoleSystem, "Need: base-dir  config.yaml")
								return
							}
							go runTUIRunChain(ctx, t, parts[0], parts[1])
						},
					},
				},
			},
			{Label: fmt.Sprintf("Overlays  (%d)", len(ovls)), Children: ovlItems},
			{
				Label: "System Health",
				Children: []*tui.MenuItem{
					{Label: "Health check", OnSelect: func(_ *tui.MenuItem, _ string) { runHealthInner(t, mgr, store, false) }},
					{Label: "Health check + auto-fix", OnSelect: func(_ *tui.MenuItem, _ string) { runHealthInner(t, mgr, store, true) }},
				},
			},
			{
				Label: "Prune",
				Children: []*tui.MenuItem{
					{Label: "Dry run  (preview)", OnSelect: func(_ *tui.MenuItem, _ string) { runTUIPrune(t, mgr, store, true) }},
					{
						Label:  "Prune unmounted overlays",
						Prompt: "Type 'yes' to prune",
						OnSelect: func(_ *tui.MenuItem, input string) {
							if strings.TrimSpace(input) != "yes" {
								t.AddMessage(tui.RoleSystem, "Cancelled.")
								return
							}
							runTUIPrune(t, mgr, store, false)
						},
					},
				},
			},
			{
				Label: "Garbage Collect",
				Children: []*tui.MenuItem{
					{Label: "Dry run  (preview)", OnSelect: func(_ *tui.MenuItem, _ string) { runTUIGC(ctx, t, store, true) }},
					{
						Label:  "Run GC",
						Prompt: "Type 'yes' to run GC",
						OnSelect: func(_ *tui.MenuItem, input string) {
							if strings.TrimSpace(input) != "yes" {
								t.AddMessage(tui.RoleSystem, "Cancelled.")
								return
							}
							runTUIGC(ctx, t, store, false)
						},
					},
				},
			},
			{Label: "Theme", Children: themeItems},
			{Label: "Exit", OnSelect: func(_ *tui.MenuItem, _ string) { t.Exit() }},
		},
	}
	t.OpenMenu(menu)
}

func buildOverlayItemsTyped(ctx context.Context, t *tui.TUI, mgr overlayManager, store *state.Store, ovls []*api.Overlay) []*tui.MenuItem {
	if len(ovls) == 0 {
		return []*tui.MenuItem{{Label: "<No overlays found>"}}
	}

	items := make([]*tui.MenuItem, 0, len(ovls))
	for _, o := range ovls {
		o := o
		name := o.Name

		mounted, _ := mgr.IsMounted(o)
		statusLabel := "unmounted"
		if mounted {
			statusLabel = "mounted"
		}
		if o.Locked {
			statusLabel += " 🔒"
		}
		if o.Persistent {
			statusLabel += " 📌"
		}
		if o.PinnedCommit != "" {
			statusLabel += " 📍"
		}

		items = append(items, &tui.MenuItem{
			Label: fmt.Sprintf("%-22s [%s]", name, statusLabel),
			Children: []*tui.MenuItem{
				// ── Inspect ────────────────────────────────────────────
				{
					Label: "Inspect",
					OnSelect: func(_ *tui.MenuItem, _ string) {
						mounted, _ := mgr.IsMounted(o)
						added, modified, deleted := countFileChanges(o.UpperDir, o.BaseDir)
						pin := "(none)"
						if o.PinnedCommit != "" {
							pin = o.PinnedCommit
							if len(pin) > 12 {
								pin = pin[:12]
							}
						}
						t.AddMessage(tui.RoleAssistant, fmt.Sprintf(
							"Name:         %s\nBranch:       %s\nStatus:       %s\nMount point:  %s\nBase dir:     %s\nUpper dir:    %s\nCreated:      %s\nLocked:       %v\nPersistent:   %v\nPinned:       %s\nChanges:      +%d added  ~%d modified  -%d deleted",
							o.Name,
							stringOr(o.Branch, "(none)"),
							boolLabel(mounted, "mounted", "unmounted"),
							o.MountPoint, o.BaseDir, o.UpperDir,
							o.CreatedAt.Format("2006-01-02 15:04:05"),
							o.Locked, o.Persistent, pin,
							added, modified, deleted,
						))
					},
				},
				// ── Logs ───────────────────────────────────────────────
				{
					Label: "View Logs  (last 4 KB)",
					OnSelect: func(_ *tui.MenuItem, _ string) {
						content, err := readLogTail(name, 4096)
						if err != nil {
							t.AddMessage(tui.RoleSystem, "Logs: "+err.Error())
							return
						}
						if content == "" {
							t.AddMessage(tui.RoleSystem, "No log output for "+name+".")
							return
						}
						t.AddMessage(tui.RoleAssistant, "=== Logs: "+name+" ===\n"+content)
					},
				},
				// ── Replay ─────────────────────────────────────────────
				{
					Label: "Replay  (re-run last agent command)",
					Children: []*tui.MenuItem{
						{
							Label: "Dry run  (preview what would replay)",
							OnSelect: func(_ *tui.MenuItem, _ string) {
								info, err := parseLastRun(name)
								if err != nil {
									t.AddMessage(tui.RoleSystem, "Replay dry-run: "+err.Error())
									return
								}
								if info.Agent == "" {
									t.AddMessage(tui.RoleSystem, "No previous agent command found in logs for "+name+".")
									return
								}
								t.AddMessage(tui.RoleAssistant, fmt.Sprintf(
									"Would replay in %s:\n  Agent: %s\n  Task:  %s", name, info.Agent, info.Task,
								))
							},
						},
						{
							Label:  "Replay",
							Prompt: "Type 'yes' to replay last agent command in " + name,
							OnSelect: func(_ *tui.MenuItem, input string) {
								if strings.TrimSpace(input) != "yes" {
									t.AddMessage(tui.RoleSystem, "Cancelled.")
									return
								}
								info, err := parseLastRun(name)
								if err != nil {
									t.AddMessage(tui.RoleSystem, "Replay: "+err.Error())
									return
								}
								if info.Agent == "" {
									t.AddMessage(tui.RoleSystem, "No previous agent command found in logs for "+name+".")
									return
								}
								t.StartSpinner("Replaying " + name + "…")
								exitCode, err := processRun(ctx, info.Agent, info.Task, "", o.BaseDir, name, "", 0, false, false, false)
								t.StopSpinner()
								if err != nil {
									t.AddMessage(tui.RoleSystem, "Replay failed: "+err.Error())
									return
								}
								if exitCode != 0 {
									t.AddMessage(tui.RoleSystem, fmt.Sprintf("Replay finished with exit code %d.", exitCode))
								} else {
									t.AddMessage(tui.RoleSystem, "Replay complete.")
								}
							},
						},
					},
				},
				// ── Watch ──────────────────────────────────────────────
				{
					Label: "Watch  (stream file changes – Ctrl+C to stop)",
					OnSelect: func(_ *tui.MenuItem, _ string) {
						mounted, _ := mgr.IsMounted(o)
						if !mounted {
							t.AddMessage(tui.RoleSystem, name+" is not mounted.")
							return
						}
						t.AddMessage(tui.RoleSystem, "Watching "+name+"… (Ctrl+C to stop; changes piped to output)")
						go func() {
							_ = pollWatch(ctx, o.UpperDir, o.BaseDir, 2, "simple")
						}()
					},
				},
				// ── Mount / Unmount / Restart ───────────────────────────
				{
					Label: "Mount  (remount if stopped)",
					OnSelect: func(_ *tui.MenuItem, _ string) {
						t.StartSpinner("Mounting " + name + "…")
						err := mgr.Mount(o)
						t.StopSpinner()
						if err != nil {
							t.AddMessage(tui.RoleSystem, "Failed to mount "+name+": "+err.Error())
							return
						}
						t.AddMessage(tui.RoleSystem, "Mounted "+name+".")
					},
				},
				{
					Label: "Unmount  (stop FUSE, keep data)",
					OnSelect: func(_ *tui.MenuItem, _ string) {
						t.StartSpinner("Unmounting " + name + "…")
						err := mgr.Unmount(o)
						t.StopSpinner()
						if err != nil {
							t.AddMessage(tui.RoleSystem, "Failed to unmount "+name+": "+err.Error())
							return
						}
						t.AddMessage(tui.RoleSystem, "Unmounted "+name+".")
					},
				},
				{
					Label: "Restart  (unmount + remount)",
					OnSelect: func(_ *tui.MenuItem, _ string) {
						t.StartSpinner("Restarting " + name + "…")
						err := processRestart(name)
						t.StopSpinner()
						if err != nil {
							t.AddMessage(tui.RoleSystem, "Restart failed: "+err.Error())
							return
						}
						t.AddMessage(tui.RoleSystem, "Restarted "+name+".")
					},
				},
				// ── Sync ───────────────────────────────────────────────
				{
					Label: "Sync  (pull base changes into overlay)",
					Children: []*tui.MenuItem{
						{
							Label:    "Dry run  (preview only)",
							OnSelect: func(_ *tui.MenuItem, _ string) { runTUISync(ctx, t, name, true, false) },
						},
						{
							Label:    "Sync",
							OnSelect: func(_ *tui.MenuItem, _ string) { runTUISync(ctx, t, name, false, false) },
						},
						{
							Label:    "Sync + stash uncommitted changes",
							OnSelect: func(_ *tui.MenuItem, _ string) { runTUISync(ctx, t, name, false, true) },
						},
					},
				},
				// ── Diff ───────────────────────────────────────────────
				{
					Label: "Diff  (show changed files)",
					Children: []*tui.MenuItem{
						{
							Label:    "Stat only  (counts)",
							OnSelect: func(_ *tui.MenuItem, _ string) { runTUIDiff(t, name, true) },
						},
						{
							Label:    "Full diff",
							OnSelect: func(_ *tui.MenuItem, _ string) { runTUIDiff(t, name, false) },
						},
					},
				},
				// ── Commit ─────────────────────────────────────────────
				{
					Label:  "Commit  (git commit all changes)",
					Prompt: "Enter commit message:",
					OnSelect: func(_ *tui.MenuItem, input string) {
						input = strings.TrimSpace(input)
						if input == "" {
							t.AddMessage(tui.RoleSystem, "Commit cancelled (empty message).")
							return
						}
						t.StartSpinner("Committing " + name + "…")
						err := processCommit(ctx, name, input, false)
						t.StopSpinner()
						if err != nil {
							t.AddMessage(tui.RoleSystem, "Commit failed: "+err.Error())
							return
						}
						t.AddMessage(tui.RoleSystem, "Committed changes in "+name+".")
					},
				},
				// ── Apply ──────────────────────────────────────────────
				{
					Label: "Apply  (merge back to base)",
					Children: []*tui.MenuItem{
						{
							Label:    "Dry run  (preview only)",
							OnSelect: func(_ *tui.MenuItem, _ string) { runTUIApply(ctx, t, name, true, false, false) },
						},
						{
							Label:    "Apply  (keep overlay running)",
							OnSelect: func(_ *tui.MenuItem, _ string) { runTUIApply(ctx, t, name, false, false, false) },
						},
						{
							Label:    "Apply + Stop",
							OnSelect: func(_ *tui.MenuItem, _ string) { runTUIApply(ctx, t, name, false, true, false) },
						},
						{
							Label:  "Apply + Cleanup  (delete overlay data)",
							Prompt: "Type 'yes' to apply + cleanup " + name,
							OnSelect: func(_ *tui.MenuItem, input string) {
								if strings.TrimSpace(input) != "yes" {
									t.AddMessage(tui.RoleSystem, "Cancelled.")
									return
								}
								runTUIApply(ctx, t, name, false, true, true)
							},
						},
					},
				},
				// ── Merge ──────────────────────────────────────────────
				{
					Label: "Merge  (pull another overlay's changes in)",
					Children: []*tui.MenuItem{
						{
							Label:  "Merge source → this overlay (dry run)",
							Prompt: "Enter source overlay name:",
							OnSelect: func(_ *tui.MenuItem, src string) {
								src = strings.TrimSpace(src)
								if src == "" || src == name {
									t.AddMessage(tui.RoleSystem, "Cancelled or invalid source.")
									return
								}
								runTUIMerge(ctx, t, src, name, true, false)
							},
						},
						{
							Label:  "Merge source → this overlay",
							Prompt: "Enter source overlay name:",
							OnSelect: func(_ *tui.MenuItem, src string) {
								src = strings.TrimSpace(src)
								if src == "" || src == name {
									t.AddMessage(tui.RoleSystem, "Cancelled or invalid source.")
									return
								}
								runTUIMerge(ctx, t, src, name, false, false)
							},
						},
						{
							Label:  "Merge source → this overlay (force / overwrite conflicts)",
							Prompt: "Enter source overlay name:",
							OnSelect: func(_ *tui.MenuItem, src string) {
								src = strings.TrimSpace(src)
								if src == "" || src == name {
									t.AddMessage(tui.RoleSystem, "Cancelled or invalid source.")
									return
								}
								runTUIMerge(ctx, t, src, name, false, true)
							},
						},
					},
				},
				// ── Compare ────────────────────────────────────────────
				{
					Label:  "Compare  (side-by-side vs another overlay)",
					Prompt: "Enter the other overlay name:",
					OnSelect: func(_ *tui.MenuItem, other string) {
						other = strings.TrimSpace(other)
						if other == "" || other == name {
							t.AddMessage(tui.RoleSystem, "Cancelled or invalid name.")
							return
						}
						runTUICompare(ctx, t, name, other)
					},
				},
				// ── Export ─────────────────────────────────────────────
				{
					Label: "Export  (save changes to file)",
					Children: []*tui.MenuItem{
						{
							Label:  "Export as unified diff  (.patch)",
							Prompt: "Output file path (e.g. /tmp/out.patch):",
							OnSelect: func(_ *tui.MenuItem, path string) {
								path = strings.TrimSpace(path)
								if path == "" {
									t.AddMessage(tui.RoleSystem, "Cancelled.")
									return
								}
								t.StartSpinner("Exporting diff…")
								err := exportDiff(o.UpperDir, o.BaseDir, path)
								t.StopSpinner()
								if err != nil {
									t.AddMessage(tui.RoleSystem, "Export failed: "+err.Error())
									return
								}
								t.AddMessage(tui.RoleSystem, "Exported diff to "+path+".")
							},
						},
						{
							Label:  "Export as tarball  (.tar.gz)",
							Prompt: "Output file path (e.g. /tmp/out.tar.gz):",
							OnSelect: func(_ *tui.MenuItem, path string) {
								path = strings.TrimSpace(path)
								if path == "" {
									t.AddMessage(tui.RoleSystem, "Cancelled.")
									return
								}
								t.StartSpinner("Exporting tarball…")
								err := exportTar(o.UpperDir, o.BaseDir, path)
								t.StopSpinner()
								if err != nil {
									t.AddMessage(tui.RoleSystem, "Export failed: "+err.Error())
									return
								}
								t.AddMessage(tui.RoleSystem, "Exported tarball to "+path+".")
							},
						},
					},
				},
				// ── Clone ──────────────────────────────────────────────
				{
					Label:  "Clone  (duplicate to new overlay)",
					Prompt: "New overlay name:",
					OnSelect: func(_ *tui.MenuItem, newName string) {
						newName = strings.TrimSpace(newName)
						if newName == "" || newName == name {
							t.AddMessage(tui.RoleSystem, "Cancelled or invalid name.")
							return
						}
						t.StartSpinner("Cloning " + name + " → " + newName + "…")
						err := doCloneInternal(ctx, name, newName, "")
						t.StopSpinner()
						if err != nil {
							t.AddMessage(tui.RoleSystem, "Clone failed: "+err.Error())
							return
						}
						t.AddMessage(tui.RoleSystem, "Cloned "+name+" → "+newName+".")
					},
				},
				// ── Rename ─────────────────────────────────────────────
				{
					Label:  "Rename  (must be stopped first)",
					Prompt: "New name for " + name + ":",
					OnSelect: func(_ *tui.MenuItem, newName string) {
						newName = strings.TrimSpace(newName)
						if newName == "" || newName == name {
							t.AddMessage(tui.RoleSystem, "Cancelled.")
							return
						}
						t.StartSpinner("Renaming " + name + " → " + newName + "…")
						err := doRenameInternal(ctx, name, newName)
						t.StopSpinner()
						if err != nil {
							t.AddMessage(tui.RoleSystem, "Rename failed: "+err.Error())
							return
						}
						t.AddMessage(tui.RoleSystem, "Renamed "+name+" → "+newName+".")
					},
				},
				// ── Pin / Unpin ────────────────────────────────────────
				{
					Label: "Pin  (lock to current base commit)",
					OnSelect: func(_ *tui.MenuItem, _ string) {
						err := setPin(ctx, name, "")
						if err != nil {
							t.AddMessage(tui.RoleSystem, "Pin failed: "+err.Error())
							return
						}
						t.AddMessage(tui.RoleSystem, "Pinned "+name+" to current base commit.")
					},
				},
				{
					Label: "Unpin  (remove commit pin)",
					OnSelect: func(_ *tui.MenuItem, _ string) {
						err := setUnpin(ctx, name)
						if err != nil {
							t.AddMessage(tui.RoleSystem, "Unpin failed: "+err.Error())
							return
						}
						t.AddMessage(tui.RoleSystem, "Unpinned "+name+".")
					},
				},
				// ── Lock / Unlock ──────────────────────────────────────
				{
					Label: "Lock  (protect from cleanup/prune/gc)",
					OnSelect: func(_ *tui.MenuItem, _ string) {
						err := setLock(name, true)
						if err != nil {
							t.AddMessage(tui.RoleSystem, "Lock failed: "+err.Error())
							return
						}
						t.AddMessage(tui.RoleSystem, "Locked "+name+".")
					},
				},
				{
					Label: "Unlock",
					OnSelect: func(_ *tui.MenuItem, _ string) {
						err := setLock(name, false)
						if err != nil {
							t.AddMessage(tui.RoleSystem, "Unlock failed: "+err.Error())
							return
						}
						t.AddMessage(tui.RoleSystem, "Unlocked "+name+".")
					},
				},
				// ── Stop / Cleanup ─────────────────────────────────────
				{
					Label: "Stop  (unmount + clear PID, keep data)",
					OnSelect: func(_ *tui.MenuItem, _ string) {
						t.StartSpinner("Stopping " + name + "…")
						err := processStop(ctx, name, false, false, false)
						t.StopSpinner()
						if err != nil {
							t.AddMessage(tui.RoleSystem, "Stop failed: "+err.Error())
							return
						}
						t.AddMessage(tui.RoleSystem, "Stopped "+name+".")
					},
				},
				{
					Label:  "Cleanup  (delete all overlay data)",
					Prompt: "Type 'yes' to permanently cleanup " + name,
					OnSelect: func(_ *tui.MenuItem, input string) {
						if strings.TrimSpace(input) != "yes" {
							t.AddMessage(tui.RoleSystem, "Cancelled.")
							return
						}
						t.StartSpinner("Cleaning up " + name + "…")
						err := mgr.Cleanup(o)
						t.StopSpinner()
						if err != nil {
							t.AddMessage(tui.RoleSystem, "Cleanup failed: "+err.Error())
							return
						}
						store.Delete(name) //nolint:errcheck
						t.AddMessage(tui.RoleSystem, "Cleanup complete — "+name+" removed.")
					},
				},
			},
		})
	}
	return items
}

// ─── Health ───────────────────────────────────────────────────────────────────

func runHealthInner(t *tui.TUI, mgr overlayManager, store *state.Store, fix bool) {
	t.StartSpinner("Running health check…")

	overlays, err := store.LoadAll()
	if err != nil {
		t.StopSpinner()
		t.AddMessage(tui.RoleSystem, "Failed to load overlays: "+err.Error())
		return
	}

	report := healthReport{
		Platform: runtime.GOOS,
		FuseOK:   checkFuseAvailable(),
		Overlays: len(overlays),
	}

	for _, ovl := range overlays {
		var issues []healthIssue
		if _, err := os.Stat(ovl.BaseDir); os.IsNotExist(err) {
			issues = append(issues, healthIssue{Overlay: ovl.Name, Kind: "missing_base", Message: "base dir missing: " + ovl.BaseDir})
		}
		if _, err := os.Stat(ovl.UpperDir); os.IsNotExist(err) {
			issues = append(issues, healthIssue{Overlay: ovl.Name, Kind: "missing_upper", Message: "upper dir missing: " + ovl.UpperDir})
		}

		mounted, err := mgr.IsMounted(ovl)
		if err != nil {
			issues = append(issues, healthIssue{Overlay: ovl.Name, Kind: "mount_error", Message: fmt.Sprintf("check failed: %v", err)})
		} else if !mounted {
			iss := healthIssue{Overlay: ovl.Name, Kind: "stale_mount", Message: "overlay is not mounted"}
			if fix {
				if err := mgr.Mount(ovl); err == nil {
					iss.Fixed = true
					iss.Message += " — remounted"
				} else {
					iss.Message += fmt.Sprintf(" — remount failed: %v", err)
				}
			}
			issues = append(issues, iss)
		} else if ovl.PID > 0 {
			if proc, err := os.FindProcess(ovl.PID); err != nil || !isProcessAlive(proc) {
				issues = append(issues, healthIssue{Overlay: ovl.Name, Kind: "dead_pid",
					Message: fmt.Sprintf("FUSE PID %d not running but overlay appears mounted", ovl.PID)})
			}
		}
		if len(issues) == 0 {
			report.Healthy++
		}
		report.Issues = append(report.Issues, issues...)
	}

	t.StopSpinner()

	fuseStr := "not found"
	if report.FuseOK {
		fuseStr = "available"
	}
	lines := []string{
		fmt.Sprintf("Platform:  %s", report.Platform),
		fmt.Sprintf("FUSE:      %s", fuseStr),
		fmt.Sprintf("Overlays:  %d total, %d healthy", report.Overlays, report.Healthy),
	}
	if len(report.Issues) == 0 {
		lines = append(lines, "✓ No issues detected.")
	} else {
		lines = append(lines, fmt.Sprintf("⚠ %d issue(s):", len(report.Issues)))
		for _, iss := range report.Issues {
			fixed := ""
			if iss.Fixed {
				fixed = "  [FIXED]"
			}
			lines = append(lines, fmt.Sprintf("  [%s] %s — %s%s", iss.Kind, iss.Overlay, iss.Message, fixed))
		}
	}
	t.AddMessage(tui.RoleAssistant, strings.Join(lines, "\n"))
}

// ─── Prune ────────────────────────────────────────────────────────────────────

func runTUIPrune(t *tui.TUI, mgr overlayManager, store *state.Store, dryRun bool) {
	if dryRun {
		t.StartSpinner("Calculating prune targets…")
	} else {
		t.StartSpinner("Pruning overlays…")
	}

	overlays, err := store.LoadAll()
	if err != nil {
		t.StopSpinner()
		t.AddMessage(tui.RoleSystem, "Failed to load overlays: "+err.Error())
		return
	}

	pruned, skipped := 0, 0
	for _, ovl := range overlays {
		if ovl.Persistent || ovl.Locked {
			skipped++
			continue
		}
		mounted, _ := mgr.IsMounted(ovl)
		if mounted {
			skipped++
			continue
		}
		pruned++
		if !dryRun {
			if err := mgr.Cleanup(ovl); err != nil {
				t.AddMessage(tui.RoleSystem, "Failed to cleanup "+ovl.Name+": "+err.Error())
				pruned--
				continue
			}
			store.Delete(ovl.Name) //nolint:errcheck
		}
	}
	if !dryRun {
		_ = mgr.Prune()
	}

	t.StopSpinner()
	action := "Pruned"
	if dryRun {
		action = "Would prune"
	}
	t.AddMessage(tui.RoleAssistant, fmt.Sprintf("%s %d overlay(s). Skipped %d (mounted/locked/persistent).", action, pruned, skipped))
}

// ─── GC ───────────────────────────────────────────────────────────────────────

func runTUIGC(_ context.Context, t *tui.TUI, store *state.Store, dryRun bool) {
	t.StartSpinner("Running garbage collection…")

	overlays, err := store.LoadAll()
	if err != nil {
		t.StopSpinner()
		t.AddMessage(tui.RoleSystem, "Failed to load overlays: "+err.Error())
		return
	}

	known := make(map[string]bool)
	for _, ovl := range overlays {
		known[ovl.Name] = true
	}

	cleaned := 0
	cleaned += gcDir(cfg.GetOverlaysPath(), known, "overlay data", dryRun)
	cleaned += gcDir(cfg.GetMountPath(), known, "mount point", dryRun)
	cleaned += gcLogs(cfg.GetLogsPath(), known, dryRun)
	cleaned += gcSnapshots(cfg.GetSnapshotsPath(), known, dryRun)

	t.StopSpinner()
	if cleaned == 0 {
		t.AddMessage(tui.RoleAssistant, "✓ Nothing to clean up.")
	} else if dryRun {
		t.AddMessage(tui.RoleAssistant, fmt.Sprintf("Would remove %d orphaned resource(s).", cleaned))
	} else {
		t.AddMessage(tui.RoleAssistant, fmt.Sprintf("Removed %d orphaned resource(s).", cleaned))
	}
}

// ─── Sync ─────────────────────────────────────────────────────────────────────

func runTUISync(ctx context.Context, t *tui.TUI, name string, dryRun, doStash bool) {
	t.StartSpinner("Syncing " + name + "…")
	// reuse doSync machinery through processSync helper
	err := processSync(ctx, name, dryRun, doStash)
	t.StopSpinner()
	if err != nil {
		t.AddMessage(tui.RoleSystem, "Sync failed: "+err.Error())
		return
	}
	if dryRun {
		t.AddMessage(tui.RoleAssistant, "Dry-run sync complete for "+name+".")
	} else {
		t.AddMessage(tui.RoleAssistant, "Synced "+name+" with base.")
	}
}

// ─── Diff ─────────────────────────────────────────────────────────────────────

func runTUIDiff(t *tui.TUI, name string, statOnly bool) {
	t.StartSpinner("Computing diff for " + name + "…")
	err := processDiff(name, "table", statOnly)
	t.StopSpinner()
	if err != nil {
		t.AddMessage(tui.RoleSystem, "Diff failed: "+err.Error())
	}
}

// ─── Apply helper ─────────────────────────────────────────────────────────────

func runTUIApply(ctx context.Context, t *tui.TUI, name string, dryRun, doStop, doCleanup bool) {
	label := "Applying " + name + "…"
	if dryRun {
		label = "Dry-run apply " + name + "…"
	}
	t.StartSpinner(label)
	err := processApply(ctx, name, dryRun, doStop, doCleanup)
	t.StopSpinner()
	if err != nil {
		t.AddMessage(tui.RoleSystem, "Apply failed: "+err.Error())
		return
	}
	if dryRun {
		t.AddMessage(tui.RoleAssistant, "Dry-run complete.")
	} else {
		t.AddMessage(tui.RoleAssistant, "Apply complete.")
	}
}

// ─── Merge helper ─────────────────────────────────────────────────────────────

func runTUIMerge(_ context.Context, t *tui.TUI, src, dst string, dryRun, force bool) {
	t.StartSpinner(fmt.Sprintf("Merging %s → %s…", src, dst))
	err := processMerge(src, dst, dryRun, force)
	t.StopSpinner()
	if err != nil {
		t.AddMessage(tui.RoleSystem, "Merge failed: "+err.Error())
		return
	}
	if dryRun {
		t.AddMessage(tui.RoleAssistant, "Dry-run merge complete.")
	} else {
		t.AddMessage(tui.RoleAssistant, fmt.Sprintf("Merged %s → %s.", src, dst))
	}
}

// ─── Compare helper ───────────────────────────────────────────────────────────

func runTUICompare(_ context.Context, t *tui.TUI, nameA, nameB string) {
	t.StartSpinner(fmt.Sprintf("Comparing %s ↔ %s…", nameA, nameB))

	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		t.StopSpinner()
		t.AddMessage(tui.RoleSystem, "Failed to open store: "+err.Error())
		return
	}
	ovlA, err := store.Load(nameA)
	if err != nil {
		t.StopSpinner()
		t.AddMessage(tui.RoleSystem, "Load "+nameA+": "+err.Error())
		return
	}
	ovlB, err := store.Load(nameB)
	if err != nil {
		t.StopSpinner()
		t.AddMessage(tui.RoleSystem, "Load "+nameB+": "+err.Error())
		return
	}

	changesA := scanChanges(ovlA.UpperDir, ovlA.BaseDir)
	changesB := scanChanges(ovlB.UpperDir, ovlB.BaseDir)
	result := buildComparison(nameA, nameB, changesA, changesB)
	t.StopSpinner()

	if len(result.Files) == 0 {
		t.AddMessage(tui.RoleAssistant, "No changes in either overlay.")
		return
	}
	lines := []string{fmt.Sprintf("%-40s  %-12s  %-12s  NOTE", "FILE", nameA, nameB)}
	for _, f := range result.Files {
		colA, colB := stringOr(f.StatusA, "—"), stringOr(f.StatusB, "—")
		note := ""
		if f.Both {
			if f.StatusA == f.StatusB && f.SizeA == f.SizeB {
				note = "identical"
			} else {
				note = "⚠ diverged"
			}
		}
		lines = append(lines, fmt.Sprintf("%-40s  %-12s  %-12s  %s", f.File, colA, colB, note))
	}
	lines = append(lines, fmt.Sprintf("\nOnly in %s: %d | Only in %s: %d | Both: %d", nameA, result.OnlyA, nameB, result.OnlyB, result.Both))
	t.AddMessage(tui.RoleAssistant, strings.Join(lines, "\n"))
}

// ─── Thin wrappers around existing logic functions ───────────────────────────

// processSync drives the sync flow by re-using the same logic as doSync.
func processSync(ctx context.Context, name string, dryRun, doStash bool) error {
	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}
	ovl, err := store.Load(name)
	if err != nil {
		return err
	}
	mgr, err := createOverlayManager()
	if err != nil {
		return err
	}
	mounted, err := mgr.IsMounted(ovl)
	if err != nil {
		return err
	}
	if !mounted {
		return fmt.Errorf("overlay %q is not mounted — start or restart it first", name)
	}
	gitOps := git.NewOperations()
	isGit, _ := gitOps.IsGitRepo(ctx, ovl.BaseDir)
	if isGit {
		return syncGit(ctx, ovl, mgr, gitOps, dryRun, doStash)
	}
	return syncNonGit(ctx, ovl, mgr, store, dryRun)
}

// processMerge wraps doMerge logic.
func processMerge(srcName, dstName string, dryRun, force bool) error {
	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}
	srcOvl, err := store.Load(srcName)
	if err != nil {
		return fmt.Errorf("source overlay: %w", err)
	}
	dstOvl, err := store.Load(dstName)
	if err != nil {
		return fmt.Errorf("target overlay: %w", err)
	}
	if srcOvl.UpperDir == "" {
		return fmt.Errorf("source overlay %q has no upper directory", srcName)
	}
	if dstOvl.UpperDir == "" {
		return fmt.Errorf("target overlay %q has no upper directory", dstName)
	}
	actions, err := planMerge(srcOvl.UpperDir, dstOvl.UpperDir)
	if err != nil {
		return err
	}
	if len(actions) == 0 {
		log.Info("No changes to merge from %q", srcName)
		return nil
	}
	conflicts := 0
	for _, a := range actions {
		if a.IsConflict {
			conflicts++
		}
	}
	if dryRun {
		return printMergePlan(actions, srcName, dstName, conflicts)
	}
	if conflicts > 0 && !force {
		printMergePlan(actions, srcName, dstName, conflicts) //nolint:errcheck
		return fmt.Errorf("%d conflict(s) — use force merge to overwrite", conflicts)
	}
	// Execute
	copied, deleted := 0, 0
	for _, a := range actions {
		srcPath := fmt.Sprintf("%s/%s", srcOvl.UpperDir, a.RelPath)
		dstPath := fmt.Sprintf("%s/%s", dstOvl.UpperDir, a.RelPath)
		if a.Status == "delete" {
			if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err == nil {
				f, _ := os.Create(filepath.Join(filepath.Dir(dstPath), ".wh."+filepath.Base(a.RelPath)))
				if f != nil {
					f.Close()
					deleted++
				}
			}
		} else {
			si, err := os.Stat(srcPath)
			if err == nil {
				if err := copyFile(srcPath, dstPath, si.Mode()); err == nil {
					copied++
				}
			}
		}
	}
	log.Info("Merged %d file(s) from %q into %q (%d copied, %d deleted)", copied+deleted, srcName, dstName, copied, deleted)
	return nil
}

// doCloneInternal clones an overlay (without CLI flag parsing).
func doCloneInternal(ctx context.Context, sourceName, targetName, branch string) error {
	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}
	srcOvl, err := store.Load(sourceName)
	if err != nil {
		return err
	}
	if store.Exists(targetName) {
		return fmt.Errorf("overlay %q already exists", targetName)
	}
	mgr, err := createOverlayManager()
	if err != nil {
		return err
	}
	gitOps := git.NewOperations()
	isGit, _ := gitOps.IsGitRepo(ctx, srcOvl.BaseDir)
	if branch == "" && isGit && cfg.Git.AutoBranch {
		branch = cfg.Git.BranchPrefix + targetName
	}
	opts := &api.CreateOptions{
		Name:    targetName,
		BaseDir: srcOvl.BaseDir,
		Branch:  branch,
	}
	newOvl, err := mgr.Create(opts)
	if err != nil {
		return fmt.Errorf("failed to create target overlay: %w", err)
	}
	if err := copyDir(srcOvl.UpperDir, newOvl.UpperDir); err != nil {
		mgr.Cleanup(newOvl) //nolint:errcheck
		return fmt.Errorf("failed to copy overlay data: %w", err)
	}
	if isGit && branch != "" {
		branchExists, _ := gitOps.BranchExists(ctx, srcOvl.BaseDir, branch)
		if !branchExists {
			if err := gitOps.CreateBranch(ctx, newOvl.MountPoint, branch, ""); err != nil {
				log.Warn("Failed to create branch %s: %v", branch, err)
			}
		} else {
			if err := gitOps.SwitchBranch(ctx, newOvl.MountPoint, branch); err != nil {
				log.Warn("Failed to switch to branch %s: %v", branch, err)
			}
		}
	}
	if err := store.Save(newOvl); err != nil {
		mgr.Cleanup(newOvl) //nolint:errcheck
		return fmt.Errorf("failed to save state: %w", err)
	}
	log.Info("Cloned %q -> %q", sourceName, targetName)
	return nil
}

// doRenameInternal renames an overlay (without CLI flag parsing) — mirrors doRename.
func doRenameInternal(_ context.Context, oldName, newName string) error {
	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}
	ovl, err := store.Load(oldName)
	if err != nil {
		return err
	}
	if store.Exists(newName) {
		return fmt.Errorf("overlay %q already exists", newName)
	}
	mgr, err := createOverlayManager()
	if err != nil {
		return err
	}
	mounted, err := mgr.IsMounted(ovl)
	if err != nil {
		return err
	}
	if mounted {
		return fmt.Errorf("overlay must be stopped before renaming (phantom stop %s)", oldName)
	}
	// Rename directories and state (mirrors rename.go doRename logic)
	oldOverlayDir := filepath.Join(cfg.GetOverlaysPath(), oldName)
	newOverlayDir := filepath.Join(cfg.GetOverlaysPath(), newName)
	if _, err := os.Stat(oldOverlayDir); err == nil {
		if err := os.Rename(oldOverlayDir, newOverlayDir); err != nil {
			return fmt.Errorf("failed to rename overlay directory: %w", err)
		}
	}
	oldMountDir := filepath.Join(cfg.GetMountPath(), oldName)
	newMountDir := filepath.Join(cfg.GetMountPath(), newName)
	if _, err := os.Stat(oldMountDir); err == nil {
		if err := os.Rename(oldMountDir, newMountDir); err != nil {
			return fmt.Errorf("failed to rename mount point: %w", err)
		}
	}
	oldLog := filepath.Join(cfg.GetLogsPath(), oldName+".log")
	newLog := filepath.Join(cfg.GetLogsPath(), newName+".log")
	if _, err := os.Stat(oldLog); err == nil {
		os.Rename(oldLog, newLog) //nolint:errcheck
	}
	ovl.Name = newName
	ovl.UpperDir = filepath.Join(newOverlayDir, "upper")
	ovl.MountPoint = newMountDir
	if err := store.Save(ovl); err != nil {
		return fmt.Errorf("failed to save new state: %w", err)
	}
	store.Delete(oldName) //nolint:errcheck
	log.Info("Renamed %q -> %q", oldName, newName)
	return nil
}

// setPin pins overlay to current (or given) commit.
func setPin(ctx context.Context, name, commit string) error {
	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return err
	}
	ovl, err := store.Load(name)
	if err != nil {
		return err
	}
	c := commit
	if c == "" {
		gitOps := git.NewOperations()
		if isGit, _ := gitOps.IsGitRepo(ctx, ovl.BaseDir); isGit {
			if h, err := gitOps.GetCommitHash(ctx, ovl.BaseDir); err == nil {
				c = h
			}
		}
	}
	ovl.PinnedCommit = c
	return store.Save(ovl)
}

// setUnpin removes the pin.
func setUnpin(_ context.Context, name string) error {
	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return err
	}
	ovl, err := store.Load(name)
	if err != nil {
		return err
	}
	ovl.PinnedCommit = ""
	return store.Save(ovl)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// ─── Run / Run-all / Run-chain helpers ───────────────────────────────────────
//
// These are the goroutine bodies (callers fire them with `go`).
// They redirect the global logger into the TUI for the duration of the run so
// that all log.Info / log.Warn / log.Error messages appear in the output pane.

// runTUIRun executes a single-agent run inside the TUI.
// Signature mirrors processRun: agentCmd, task, model, baseDir, name, branch,
// timeoutMinutes, doCleanup, doPush, persist.
func runTUIRun(ctx context.Context, t *tui.TUI, baseDir, agentCmd, task, model, name, branch string, timeoutMinutes int, doCleanup, doPush, persist bool) {
	label := fmt.Sprintf("Running agent %q in %s…", agentCmd, baseDir)
	t.StartSpinner(label)

	// Temporarily redirect global logger to TUI
	oldLog := log
	log = &tuiLogger{t: t}
	exitCode, err := processRun(ctx, agentCmd, task, model, baseDir, name, branch, timeoutMinutes, doCleanup, doPush, persist)
	log = oldLog

	t.StopSpinner()
	if err != nil {
		t.AddMessage(tui.RoleSystem, fmt.Sprintf("run failed: %v", err))
		return
	}
	if exitCode != 0 {
		t.AddMessage(tui.RoleSystem, fmt.Sprintf("Agent exited with code %d.", exitCode))
	} else {
		t.AddMessage(tui.RoleSystem, fmt.Sprintf("Agent %q finished successfully.", agentCmd))
	}
}

// runTUIRunAll loads an agents config file and runs all agents in parallel.
func runTUIRunAll(ctx context.Context, t *tui.TUI, baseDir, configPath string) {
	t.StartSpinner(fmt.Sprintf("Loading agents from %s…", configPath))
	agents, err := loadAgentsConfig(configPath)
	t.StopSpinner()
	if err != nil {
		t.AddMessage(tui.RoleSystem, "run-all: failed to load config: "+err.Error())
		return
	}
	t.AddMessage(tui.RoleSystem, fmt.Sprintf("Starting %d agent(s) in parallel…", len(agents)))

	oldLog := log
	log = &tuiLogger{t: t}
	err = processRunAll(ctx, baseDir, agents, 0, false, false, "table")
	log = oldLog

	if err != nil {
		t.AddMessage(tui.RoleSystem, "run-all failed: "+err.Error())
	} else {
		t.AddMessage(tui.RoleSystem, fmt.Sprintf("run-all complete (%d agent(s)).", len(agents)))
	}
}

// runTUIRunChain loads a chain config file and runs steps sequentially.
func runTUIRunChain(ctx context.Context, t *tui.TUI, baseDir, configPath string) {
	t.StartSpinner(fmt.Sprintf("Loading chain from %s…", configPath))
	chainCfg, err := loadChainConfig(configPath)
	t.StopSpinner()
	if err != nil {
		t.AddMessage(tui.RoleSystem, "run-chain: failed to load config: "+err.Error())
		return
	}
	steps := chainCfg.Steps
	if len(steps) == 0 {
		t.AddMessage(tui.RoleSystem, "run-chain: no steps defined in config.")
		return
	}
	t.AddMessage(tui.RoleSystem, fmt.Sprintf("Running chain %q (%d step(s))…", chainCfg.Name, len(steps)))

	oldLog := log
	log = &tuiLogger{t: t}
	err = processRunChain(ctx, baseDir, chainCfg.Name, chainCfg.Branch, steps, 0, false, false, false, "table")
	log = oldLog

	if err != nil {
		t.AddMessage(tui.RoleSystem, "run-chain failed: "+err.Error())
	} else {
		t.AddMessage(tui.RoleSystem, fmt.Sprintf("Chain %q complete.", chainCfg.Name))
	}
}

// runTUIStart creates and mounts a new overlay from within the TUI.
func runTUIStart(ctx context.Context, t *tui.TUI, baseDir, name, branch string, persistent bool) {
	if baseDir == "" {
		t.AddMessage(tui.RoleSystem, "Base directory is required.")
		return
	}
	label := "Starting overlay"
	if name != "" {
		label += " " + name
	}
	label += "…"
	t.StartSpinner(label)
	err := processStart(ctx, baseDir, name, branch, persistent)
	t.StopSpinner()
	if err != nil {
		t.AddMessage(tui.RoleSystem, "Start failed: "+err.Error())
		return
	}
	ovlName := name
	if ovlName == "" {
		// processStart auto-generates from the base dir name
		ovlName = filepath.Base(baseDir)
	}
	t.AddMessage(tui.RoleSystem, "Overlay "+ovlName+" started. Re-open /menu to see it in the Overlays list.")
}

func readLogTail(name string, maxBytes int64) (string, error) {
	logPath := filepath.Join(cfg.GetLogsPath(), name+".log")
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no logs found for overlay %q", name)
		}
		return "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if offset := info.Size() - maxBytes; offset > 0 {
		f.Seek(offset, io.SeekStart)
		// Skip partial first line
		buf := make([]byte, 1)
		for {
			if _, err := f.Read(buf); err != nil || buf[0] == '\n' {
				break
			}
		}
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func stringOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func boolLabel(v bool, trueStr, falseStr string) string {
	if v {
		return trueStr
	}
	return falseStr
}
