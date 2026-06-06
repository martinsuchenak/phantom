package commands

import (
	"path/filepath"
	"testing"

	"github.com/martinsuchenak/phantom/internal/config"
	"github.com/paularlott/cli"
)

func TestNewCommands(t *testing.T) {
	tmpDir := t.TempDir()
	oldCfg := cfg
	oldLog := log
	defer func() {
		cfg = oldCfg
		log = oldLog
	}()

	cfg = &config.Config{
		StateDir: filepath.Join(tmpDir, "state"),
	}
	log = &MockLogger{}

	_, cleanup := setupMockPath(t)
	defer cleanup()

	commands := []struct {
		name string
		f    func() *cli.Command
	}{
		{"run", NewRunCommand},
		{"start", NewStartCommand},
		{"status", NewStatusCommand},
		{"stop", NewStopCommand},
		{"logs", NewLogsCommand},
		{"list", NewListCommand},
		{"diff", NewDiffCommand},
		{"export", NewExportCommand},
		{"gc", NewGCCommand},
		{"hook", NewHookCommand},
		{"init", NewInitCommand},
		{"lock", NewLockCommand},
		{"pin", NewPinCommand},
		{"prune", NewPruneCommand},
		{"rename", NewRenameCommand},
		{"replay", NewReplayCommand},
		{"restart", NewRestartCommand},
		{"run-all", NewRunAllCommand},
		{"run-chain", NewRunChainCommand},
		{"snapshot", NewSnapshotCommand},
		{"sync", NewSyncCommand},
		{"template", NewTemplateCommand},
		{"watch", NewWatchCommand},
		{"health", NewHealthCommand},
		{"inspect", NewInspectCommand},
		{"merge", NewMergeCommand},
		{"apply", NewApplyCommand},
		{"compare", NewCompareCommand},
		{"conflicts", NewConflictsCommand},
	}

	for _, cmd := range commands {
		t.Run(cmd.name, func(t *testing.T) {
			c := cmd.f()
			if c == nil {
				t.Fatalf("expected New%sCommand to return a valid command, got nil", cmd.name)
			}
			if c.Name != cmd.name {
				t.Errorf("expected command name %q, got %q", cmd.name, c.Name)
			}
			// We only verify construction; executing Run with unparsed flags would cause side effects.
		})
	}
}
