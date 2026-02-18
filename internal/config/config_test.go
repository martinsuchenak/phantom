package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("expected config to not be nil")
	}

	// Check default values
	if cfg.Overlay.Persistent {
		t.Error("expected Persistent to be false by default")
	}
	if cfg.Overlay.AutoCleanupDays != 7 {
		t.Errorf("expected AutoCleanupDays to be 7, got %d", cfg.Overlay.AutoCleanupDays)
	}
	if !cfg.Git.AutoBranch {
		t.Error("expected AutoBranch to be true by default")
	}
	if cfg.Git.BranchPrefix != "overlay/" {
		t.Errorf("expected BranchPrefix to be 'overlay/', got %s", cfg.Git.BranchPrefix)
	}
	if cfg.Agent.DefaultTimeoutMinutes != 60 {
		t.Errorf("expected DefaultTimeoutMinutes to be 60, got %d", cfg.Agent.DefaultTimeoutMinutes)
	}
	if !cfg.Agent.CleanupOnSuccess {
		t.Error("expected CleanupOnSuccess to be true by default")
	}
	if cfg.Agent.CleanupOnFailure {
		t.Error("expected CleanupOnFailure to be false by default")
	}
}

func TestLoadNonExistentConfig(t *testing.T) {
	// Load should return defaults when file doesn't exist
	cfg, err := Load("/non/existent/path/config.yaml")
	if err != nil {
		t.Fatalf("expected no error for non-existent config, got: %v", err)
	}

	if cfg == nil {
		t.Fatal("expected config to not be nil")
	}

	// Should have default values
	if !cfg.Git.AutoBranch {
		t.Error("expected AutoBranch to be true (default)")
	}
}

func TestLoadAndSaveConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-config-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create custom config
	cfg := DefaultConfig()
	cfg.Overlay.Persistent = true
	cfg.Overlay.AutoCleanupDays = 14
	cfg.Git.BranchPrefix = "phantom/"
	cfg.Agent.DefaultTimeoutMinutes = 120

	// Save
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Load
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Verify
	if !loaded.Overlay.Persistent {
		t.Error("expected Persistent to be true")
	}
	if loaded.Overlay.AutoCleanupDays != 14 {
		t.Errorf("expected AutoCleanupDays to be 14, got %d", loaded.Overlay.AutoCleanupDays)
	}
	if loaded.Git.BranchPrefix != "phantom/" {
		t.Errorf("expected BranchPrefix to be 'phantom/', got %s", loaded.Git.BranchPrefix)
	}
	if loaded.Agent.DefaultTimeoutMinutes != 120 {
		t.Errorf("expected DefaultTimeoutMinutes to be 120, got %d", loaded.Agent.DefaultTimeoutMinutes)
	}
}

func TestConfigPaths(t *testing.T) {
	cfg := DefaultConfig()

	statePath := cfg.GetStatePath()
	if statePath == "" {
		t.Error("expected StatePath to not be empty")
	}

	overlaysPath := cfg.GetOverlaysPath()
	if overlaysPath == "" {
		t.Error("expected OverlaysPath to not be empty")
	}

	mountPath := cfg.GetMountPath()
	if mountPath == "" {
		t.Error("expected MountPath to not be empty")
	}

	// Verify paths contain state dir
	if !filepath.IsAbs(statePath) {
		t.Errorf("expected StatePath to be absolute, got %s", statePath)
	}
}

func TestExpandHome(t *testing.T) {
	homeDir, _ := os.UserHomeDir()

	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "~/test",
			expected: filepath.Join(homeDir, "test"),
		},
		{
			input:    "~",
			expected: homeDir,
		},
		{
			input:    "/absolute/path",
			expected: "/absolute/path",
		},
		{
			input:    "relative/path",
			expected: "relative/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := expandHome(tt.input)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestConfigWithDarwinSettings(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-config-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create config with Darwin settings
	cfg := DefaultConfig()
	cfg.Darwin.UnionFSPath = "/opt/homebrew/bin/unionfs-fuse"
	cfg.Darwin.FUSEOptions = []string{"cow", "noatime"}

	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loaded.Darwin.UnionFSPath != "/opt/homebrew/bin/unionfs-fuse" {
		t.Errorf("expected UnionFSPath '/opt/homebrew/bin/unionfs-fuse', got %s", loaded.Darwin.UnionFSPath)
	}
	if len(loaded.Darwin.FUSEOptions) != 2 {
		t.Errorf("expected 2 FUSE options, got %d", len(loaded.Darwin.FUSEOptions))
	}
}

func TestConfigWithAgentEnv(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-config-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := DefaultConfig()
	cfg.AgentEnv = []string{
		"OVERLAY_ENABLED=true",
		"AGENT_MODE=development",
	}

	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if len(loaded.AgentEnv) != 2 {
		t.Errorf("expected 2 agent env vars, got %d", len(loaded.AgentEnv))
	}
}
