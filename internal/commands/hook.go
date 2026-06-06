package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/paularlott/cli"
	"gopkg.in/yaml.v3"
)

// HookDef defines a post-run hook
type HookDef struct {
	Name    string `yaml:"name" json:"name"`
	On      string `yaml:"on" json:"on"`           // "success", "failure", "always"
	Command string `yaml:"command" json:"command"` // shell command to run
}

// HooksConfig is the YAML structure for hooks file
type HooksConfig struct {
	Hooks []HookDef `yaml:"hooks"`
}

// NewHookCommand creates the hook command
func NewHookCommand() *cli.Command {
	return &cli.Command{
		Name:  "hook",
		Usage: "Manage post-run hooks",
		Description: "Hooks run automatically after an agent finishes in an overlay. " +
			"Configure hooks in ~/.phantom/hooks.yaml or per-overlay.",
		Commands: []*cli.Command{
			{
				Name:  "list",
				Usage: "List configured hooks",
				Run:   doHookList,
			},
			{
				Name:  "add",
				Usage: "Add a new hook",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "name", Usage: "Hook name", Required: true},
					&cli.StringFlag{Name: "on", Usage: "When to run: success, failure, always", DefaultValue: "success"},
					&cli.StringFlag{Name: "command", Aliases: []string{"cmd"}, Usage: "Command to execute", Required: true},
				},
				Run: doHookAdd,
			},
			{
				Name:  "remove",
				Usage: "Remove a hook by name",
				Arguments: []cli.Argument{
					&cli.StringArg{Name: "name", Usage: "Hook name to remove", Required: true},
				},
				Run: doHookRemove,
			},
			{
				Name:  "init",
				Usage: "Create example hooks.yaml",
				Run:   doHookInit,
			},
		},
	}
}

func hooksFilePath() string {
	return filepath.Join(cfg.GetStatePath(), "hooks.yaml")
}

func loadHooks() (*HooksConfig, error) {
	path := hooksFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &HooksConfig{}, nil
		}
		return nil, err
	}
	var hc HooksConfig
	if err := yaml.Unmarshal(data, &hc); err != nil {
		return nil, fmt.Errorf("invalid hooks.yaml: %w", err)
	}
	return &hc, nil
}

func saveHooks(hc *HooksConfig) error {
	path := hooksFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := yaml.Marshal(hc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func doHookList(ctx context.Context, cmd *cli.Command) error {
	hc, err := loadHooks()
	if err != nil {
		return err
	}
	if len(hc.Hooks) == 0 {
		log.Info("No hooks configured. Use 'phantom hook add' or 'phantom hook init' to get started.")
		return nil
	}
	for _, h := range hc.Hooks {
		fmt.Printf("  %s  [on:%s]  %s\n", h.Name, h.On, h.Command)
	}
	return nil
}

func doHookAdd(ctx context.Context, cmd *cli.Command) error {
	name := cmd.GetString("name")
	on := cmd.GetString("on")
	command := cmd.GetString("command")

	if on != "success" && on != "failure" && on != "always" {
		return fmt.Errorf("--on must be one of: success, failure, always")
	}

	hc, err := loadHooks()
	if err != nil {
		return err
	}

	// Check for duplicate
	for _, h := range hc.Hooks {
		if h.Name == name {
			return fmt.Errorf("hook %q already exists — remove it first", name)
		}
	}

	hc.Hooks = append(hc.Hooks, HookDef{
		Name:    name,
		On:      on,
		Command: command,
	})

	if err := saveHooks(hc); err != nil {
		return err
	}
	log.Info("Added hook %q (on:%s)", name, on)
	return nil
}

func doHookRemove(ctx context.Context, cmd *cli.Command) error {
	name := cmd.GetStringArg("name")

	hc, err := loadHooks()
	if err != nil {
		return err
	}

	found := false
	var remaining []HookDef
	for _, h := range hc.Hooks {
		if h.Name == name {
			found = true
			continue
		}
		remaining = append(remaining, h)
	}

	if !found {
		return fmt.Errorf("hook %q not found", name)
	}

	hc.Hooks = remaining
	if err := saveHooks(hc); err != nil {
		return err
	}
	log.Info("Removed hook %q", name)
	return nil
}

func doHookInit(ctx context.Context, cmd *cli.Command) error {
	path := hooksFilePath()
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("hooks.yaml already exists at %s", path)
	}

	example := `# Phantom post-run hooks
# Hooks run automatically after an agent finishes in an overlay.
#
# on: success | failure | always
# command: shell command to run (executed in the overlay mount point)
#
# Available environment variables:
#   OVERLAY_NAME, OVERLAY_PATH, OVERLAY_BASE, OVERLAY_BRANCH,
#   OVERLAY_EXIT_CODE, OVERLAY_AGENT, OVERLAY_TASK

hooks:
  - name: lint
    on: success
    command: "npm run lint --fix 2>/dev/null || true"
  - name: test
    on: success
    command: "go test ./... 2>&1 | tail -5"
  - name: notify-failure
    on: failure
    command: "echo 'Agent failed in $OVERLAY_NAME' >> ~/.phantom/failures.log"
`
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(example), 0600); err != nil {
		return err
	}
	log.Info("Created %s with example hooks", path)
	return nil
}

// ExecuteHooks runs all matching hooks after an agent completes.
// Called from the agent runner or run commands.
func ExecuteHooks(overlayName, mountPoint, baseDir, branch, agentCmd, task string, exitCode int) {
	if cfg == nil {
		return
	}

	hc, err := loadHooks()
	if err != nil || len(hc.Hooks) == 0 {
		return
	}

	success := exitCode == 0
	for _, h := range hc.Hooks {
		shouldRun := h.On == "always" ||
			(h.On == "success" && success) ||
			(h.On == "failure" && !success)

		if !shouldRun {
			continue
		}

		log.Debug("Running hook %q: %s", h.Name, h.Command)
		runHookCommand(h, mountPoint, baseDir, branch, overlayName, agentCmd, task, exitCode)
	}
}

func runHookCommand(h HookDef, mountPoint, baseDir, branch, name, agentCmd, task string, exitCode int) {
	cmd := exec.Command("sh", "-c", h.Command)
	cmd.Dir = mountPoint
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("OVERLAY_NAME=%s", name),
		fmt.Sprintf("OVERLAY_PATH=%s", mountPoint),
		fmt.Sprintf("OVERLAY_BASE=%s", baseDir),
		fmt.Sprintf("OVERLAY_BRANCH=%s", branch),
		fmt.Sprintf("OVERLAY_EXIT_CODE=%d", exitCode),
		fmt.Sprintf("OVERLAY_AGENT=%s", agentCmd),
		fmt.Sprintf("OVERLAY_TASK=%s", task),
	)

	if err := cmd.Run(); err != nil {
		log.Warn("Hook %q failed: %v", h.Name, err)
	}
}

// RunHooksForOverlay is a convenience wrapper for running hooks after agent completion.
// It loads hooks config and executes matching hooks.
func RunHooksForOverlay(name, mountPoint, baseDir, branch, agentCmd, task string, exitCode int) {
	ExecuteHooks(name, mountPoint, baseDir, branch, agentCmd, task, exitCode)
}
