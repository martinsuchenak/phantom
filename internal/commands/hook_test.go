package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewHookCommand(t *testing.T) {
	cmd := NewHookCommand()
	if cmd.Name != "hook" {
		t.Errorf("expected command name 'hook', got %q", cmd.Name)
	}
	if len(cmd.Commands) != 4 {
		t.Errorf("expected 4 subcommands, got %d", len(cmd.Commands))
	}
}

func TestLoadHooksEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	hc, err := loadHooks()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hc.Hooks) != 0 {
		t.Errorf("expected 0 hooks, got %d", len(hc.Hooks))
	}
}

func TestSaveAndLoadHooks(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	hc := &HooksConfig{
		Hooks: []HookDef{
			{Name: "lint", On: "success", Command: "npm run lint"},
			{Name: "notify", On: "failure", Command: "echo failed"},
		},
	}

	if err := saveHooks(hc); err != nil {
		t.Fatalf("failed to save hooks: %v", err)
	}

	loaded, err := loadHooks()
	if err != nil {
		t.Fatalf("failed to load hooks: %v", err)
	}
	if len(loaded.Hooks) != 2 {
		t.Fatalf("expected 2 hooks, got %d", len(loaded.Hooks))
	}
	if loaded.Hooks[0].Name != "lint" {
		t.Errorf("expected hook name 'lint', got %q", loaded.Hooks[0].Name)
	}
	if loaded.Hooks[1].On != "failure" {
		t.Errorf("expected hook on 'failure', got %q", loaded.Hooks[1].On)
	}
}

func TestLoadHooksInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	hooksPath := hooksFilePath()
	_ = os.MkdirAll(filepath.Dir(hooksPath), 0700)
	_ = os.WriteFile(hooksPath, []byte("{{invalid"), 0600)

	_, err := loadHooks()
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestExecuteHooksFiltering(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	// Create a hooks file with a simple echo command
	marker := filepath.Join(tmpDir, "hook-ran")
	hc := &HooksConfig{
		Hooks: []HookDef{
			{Name: "on-success", On: "success", Command: "touch " + marker},
			{Name: "on-failure", On: "failure", Command: "touch " + marker + "-fail"},
		},
	}
	_ = saveHooks(hc)

	t.Run("success hooks run on exit 0", func(t *testing.T) {
		_ = os.Remove(marker)
		ExecuteHooks("test", tmpDir, tmpDir, "", "agent", "task", 0)
		if _, err := os.Stat(marker); os.IsNotExist(err) {
			t.Error("expected success hook to run")
		}
	})

	t.Run("failure hooks run on non-zero exit", func(t *testing.T) {
		_ = os.Remove(marker + "-fail")
		ExecuteHooks("test", tmpDir, tmpDir, "", "agent", "task", 1)
		if _, err := os.Stat(marker + "-fail"); os.IsNotExist(err) {
			t.Error("expected failure hook to run")
		}
	})

	t.Run("success hooks don't run on failure", func(t *testing.T) {
		_ = os.Remove(marker)
		ExecuteHooks("test", tmpDir, tmpDir, "", "agent", "task", 1)
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Error("expected success hook NOT to run on failure")
		}
	})
}

func TestExecuteHooksAlways(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	marker := filepath.Join(tmpDir, "always-ran")
	hc := &HooksConfig{
		Hooks: []HookDef{
			{Name: "always", On: "always", Command: "touch " + marker},
		},
	}
	_ = saveHooks(hc)

	_ = os.Remove(marker)
	ExecuteHooks("test", tmpDir, tmpDir, "", "agent", "task", 1)
	if _, err := os.Stat(marker); os.IsNotExist(err) {
		t.Error("expected 'always' hook to run on failure")
	}

	_ = os.Remove(marker)
	ExecuteHooks("test", tmpDir, tmpDir, "", "agent", "task", 0)
	if _, err := os.Stat(marker); os.IsNotExist(err) {
		t.Error("expected 'always' hook to run on success")
	}
}

func TestExecuteHooksNilConfig(t *testing.T) {
	oldCfg := cfg
	cfg = nil
	defer func() { cfg = oldCfg }()

	// Should not panic
	ExecuteHooks("test", "/tmp", "/tmp", "", "agent", "task", 0)
}
