package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/paularlott/cli"
)

// NewInitCommand creates the init command
func NewInitCommand() *cli.Command {
	return &cli.Command{
		Name:        "init",
		Usage:       "Initialize Phantom configuration",
		Description: "Creates default config.yaml and an example agents.yaml in ~/.phantom/. Existing files are not overwritten unless --force is used.",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "Overwrite existing files",
			},
		},
		Run: doInit,
	}
}

const defaultConfigYAML = `# Phantom configuration
# See: https://github.com/martinsuchenak/phantom

state_dir: "~/.phantom"

logging:
  level: info                      # trace, debug, info, warn, error, fatal
  file: "~/.phantom/phantom.log"

overlay:
  persistent: false
  auto_cleanup_days: 7             # 0 to disable auto-cleanup

git:
  auto_branch: true
  branch_prefix: "phantom/"
  auto_push_on_stop: false

darwin:
  unionfs_path: ""                 # auto-detect
  fuse_options:
    - "cow"

linux:
  use_fuse: false                  # auto-detects if not root
  fuse_overlay_path: ""            # auto-detect fuse-overlayfs
  fuse_options: []

agent:
  default_timeout_minutes: 60      # max: 1440 (24 hours)
  cleanup_on_success: true
  cleanup_on_failure: false

agent_env:
  - "OVERLAY_ENABLED=true"
`

const exampleAgentsYAML = `# Example agents.yaml for phantom run-all
# Usage: phantom run-all /path/to/repo --config agents.yaml

agents:
  - name: auth-agent
    agent: "claude --print"
    task: "implement authentication module"
    branch: "feature/auth"
    timeout: 30

  - name: api-agent
    agent: 'aider "{task}"'
    task: "build REST API endpoints"
    branch: "feature/api"

  - name: test-agent
    agent: 'gemini "{task}"'
    task: "write unit tests for all modules"
    branch: "feature/tests"
`

func doInit(ctx context.Context, cmd *cli.Command) error {
	force := cmd.GetBool("force")

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	phantomDir := filepath.Join(homeDir, ".phantom")
	if err := os.MkdirAll(phantomDir, 0700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	configPath := filepath.Join(phantomDir, "config.yaml")
	agentsPath := filepath.Join(phantomDir, "agents.yaml")

	wrote := 0

	if err := writeIfNotExists(configPath, defaultConfigYAML, force); err != nil {
		return err
	} else {
		wrote++
	}

	if err := writeIfNotExists(agentsPath, exampleAgentsYAML, force); err != nil {
		return err
	} else {
		wrote++
	}

	if wrote > 0 {
		log.Info("Initialized Phantom config in %s", phantomDir)
	}

	return nil
}

func writeIfNotExists(path, content string, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			log.Info("Skipping %s (already exists, use --force to overwrite)", filepath.Base(path))
			return nil
		}
	}

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}

	log.Info("Created %s", path)
	return nil
}
