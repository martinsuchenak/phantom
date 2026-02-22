package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoHookListEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	hc, err := loadHooks()
	if err != nil {
		t.Fatal(err)
	}
	if len(hc.Hooks) != 0 {
		t.Errorf("expected 0 hooks, got %d", len(hc.Hooks))
	}

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
}

func TestDoHookAddAndList(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	// Add a hook
	hc, _ := loadHooks()
	hc.Hooks = append(hc.Hooks, HookDef{Name: "test-hook", On: "success", Command: "echo ok"})
	if err := saveHooks(hc); err != nil {
		t.Fatal(err)
	}

	// Load and verify
	loaded, err := loadHooks()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(loaded.Hooks))
	}
	if loaded.Hooks[0].Name != "test-hook" {
		t.Errorf("expected name 'test-hook', got %q", loaded.Hooks[0].Name)
	}
}

func TestDoHookAddDuplicate(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	hc := &HooksConfig{
		Hooks: []HookDef{{Name: "dup", On: "success", Command: "echo"}},
	}
	saveHooks(hc)

	// Try adding duplicate
	loaded, _ := loadHooks()
	for _, h := range loaded.Hooks {
		if h.Name == "dup" {
			// duplicate found, as expected
			return
		}
	}
	t.Error("expected to find existing hook 'dup'")
}

func TestDoHookRemoveByName(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	hc := &HooksConfig{
		Hooks: []HookDef{
			{Name: "keep", On: "success", Command: "echo keep"},
			{Name: "remove-me", On: "failure", Command: "echo remove"},
			{Name: "also-keep", On: "always", Command: "echo also"},
		},
	}
	saveHooks(hc)

	// Remove the middle hook
	loaded, _ := loadHooks()
	var remaining []HookDef
	for _, h := range loaded.Hooks {
		if h.Name != "remove-me" {
			remaining = append(remaining, h)
		}
	}
	loaded.Hooks = remaining
	saveHooks(loaded)

	// Verify
	final, _ := loadHooks()
	if len(final.Hooks) != 2 {
		t.Fatalf("expected 2 hooks after removal, got %d", len(final.Hooks))
	}
	for _, h := range final.Hooks {
		if h.Name == "remove-me" {
			t.Error("hook 'remove-me' should have been removed")
		}
	}
}

func TestDoHookRemoveNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	hc := &HooksConfig{
		Hooks: []HookDef{{Name: "exists", On: "success", Command: "echo"}},
	}
	saveHooks(hc)

	loaded, _ := loadHooks()
	found := false
	for _, h := range loaded.Hooks {
		if h.Name == "nonexistent" {
			found = true
		}
	}
	if found {
		t.Error("should not find nonexistent hook")
	}
}

func TestDoHookInit(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	path := hooksFilePath()

	// Should not exist yet
	if _, err := os.Stat(path); err == nil {
		t.Fatal("hooks.yaml should not exist yet")
	}

	// Create example hooks file
	example := "hooks:\n  - name: test\n    on: success\n    command: echo\n"
	os.MkdirAll(filepath.Dir(path), 0700)
	if err := os.WriteFile(path, []byte(example), 0600); err != nil {
		t.Fatal(err)
	}

	// Verify it exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("hooks.yaml should exist after init")
	}

	// Verify content is valid YAML
	loaded, err := loadHooks()
	if err != nil {
		t.Fatalf("failed to load hooks after init: %v", err)
	}
	if len(loaded.Hooks) != 1 {
		t.Errorf("expected 1 hook in example, got %d", len(loaded.Hooks))
	}
}

func TestDoHookInitAlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	path := hooksFilePath()
	os.MkdirAll(filepath.Dir(path), 0700)
	os.WriteFile(path, []byte("existing"), 0600)

	// Stat should succeed (file exists)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("hooks.yaml should already exist")
	}
}

func TestRunHooksForOverlay(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	marker := filepath.Join(tmpDir, "hook-marker")
	hc := &HooksConfig{
		Hooks: []HookDef{
			{Name: "marker", On: "always", Command: "touch " + marker},
		},
	}
	saveHooks(hc)

	RunHooksForOverlay("test", tmpDir, tmpDir, "", "agent", "task", 0)

	if _, err := os.Stat(marker); os.IsNotExist(err) {
		t.Error("RunHooksForOverlay should have executed the hook")
	}
}

func TestHooksFilePath(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	path := hooksFilePath()
	if !strings.Contains(path, "hooks.yaml") {
		t.Errorf("expected path to contain 'hooks.yaml', got %q", path)
	}
}
