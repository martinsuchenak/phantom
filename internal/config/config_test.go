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
	if cfg.Git.BranchPrefix != "phantom/" {
		t.Errorf("expected BranchPrefix to be 'phantom/', got %s", cfg.Git.BranchPrefix)
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


func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		modify      func(*Config)
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid config",
			modify:      func(c *Config) {},
			expectError: false,
		},
		{
			name: "invalid log level",
			modify: func(c *Config) {
				c.Logging.Level = "invalid"
			},
			expectError: true,
			errorMsg:    "invalid log level",
		},
		{
			name: "negative timeout",
			modify: func(c *Config) {
				c.Agent.DefaultTimeoutMinutes = -1
			},
			expectError: true,
			errorMsg:    "cannot be negative",
		},
		{
			name: "timeout too large",
			modify: func(c *Config) {
				c.Agent.DefaultTimeoutMinutes = MaxTimeoutMinutes + 1
			},
			expectError: true,
			errorMsg:    "cannot exceed",
		},
		{
			name: "negative cleanup days",
			modify: func(c *Config) {
				c.Overlay.AutoCleanupDays = -1
			},
			expectError: true,
			errorMsg:    "cannot be negative",
		},
		{
			name: "empty state dir",
			modify: func(c *Config) {
				c.StateDir = ""
			},
			expectError: true,
			errorMsg:    "cannot be empty",
		},
		{
			name: "valid log levels",
			modify: func(c *Config) {
				c.Logging.Level = "debug"
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modify(cfg)

			err := cfg.Validate()
			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestNodeConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Node.GossipPort != 7946 {
		t.Errorf("expected gossip port 7946, got %d", cfg.Node.GossipPort)
	}
	if cfg.Node.GRPCPort != 50051 {
		t.Errorf("expected grpc port 50051, got %d", cfg.Node.GRPCPort)
	}
	if cfg.Node.Auth.Mode != "none" {
		t.Errorf("expected auth mode 'none', got %q", cfg.Node.Auth.Mode)
	}
	if cfg.Node.Sync.MaxFileSizeBytes != 50*1024*1024 {
		t.Errorf("expected max file size 50MB, got %d", cfg.Node.Sync.MaxFileSizeBytes)
	}
}

func TestNodeConfigGetRemoteMountsPath(t *testing.T) {
	cfg := DefaultConfig()
	path := cfg.GetRemoteMountsPath()
	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got %q", path)
	}
}

func TestNodeConfigGetNodePIDPath(t *testing.T) {
	cfg := DefaultConfig()
	path := cfg.GetNodePIDPath()
	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got %q", path)
	}
}

func TestNodeConfigGetPeersStatePath(t *testing.T) {
	cfg := DefaultConfig()
	path := cfg.GetPeersStatePath()
	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got %q", path)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestConfigFilePermissions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-config-perm-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := DefaultConfig()
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Check file permissions
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("failed to stat config file: %v", err)
	}

	if info.Mode().Perm() != 0600 {
		t.Errorf("expected file permissions 0600, got %v", info.Mode().Perm())
	}
}


func TestGetLogsPath(t *testing.T) {
	cfg := DefaultConfig()
	logsPath := cfg.GetLogsPath()
	if logsPath == "" {
		t.Error("expected non-empty logs path")
	}

	// Test with custom path
	cfg.Paths.Logs = "/custom/logs"
	if cfg.GetLogsPath() != "/custom/logs" {
		t.Errorf("expected /custom/logs, got %s", cfg.GetLogsPath())
	}
}

func TestGetSnapshotsPath(t *testing.T) {
	cfg := DefaultConfig()
	snapsPath := cfg.GetSnapshotsPath()
	if snapsPath == "" {
		t.Error("expected non-empty snapshots path")
	}

	// Test with custom path
	cfg.Paths.Snapshots = "/custom/snapshots"
	if cfg.GetSnapshotsPath() != "/custom/snapshots" {
		t.Errorf("expected /custom/snapshots, got %s", cfg.GetSnapshotsPath())
	}
}

func TestGetOverlaysPath_Custom(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Paths.Overlays = "/custom/overlays"
	if cfg.GetOverlaysPath() != "/custom/overlays" {
		t.Errorf("expected /custom/overlays, got %s", cfg.GetOverlaysPath())
	}
}

func TestGetMountPath_Custom(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Paths.Mounts = "/custom/mounts"
	if cfg.GetMountPath() != "/custom/mounts" {
		t.Errorf("expected /custom/mounts, got %s", cfg.GetMountPath())
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-config-invalid-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")
	os.WriteFile(configPath, []byte("{{invalid yaml"), 0644)

	_, err = Load(configPath)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestLoadInvalidConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-config-badval-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")
	// Valid YAML but invalid config (bad log level)
	os.WriteFile(configPath, []byte("logging:\n  level: invalid_level\n"), 0644)

	_, err = Load(configPath)
	if err == nil {
		t.Error("expected error for invalid log level")
	}
}

func TestSaveCreatesDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-config-savedir-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := DefaultConfig()
	configPath := filepath.Join(tmpDir, "subdir", "config.yaml")
	err = cfg.Save(configPath)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file should exist")
	}
}

func TestProjectUnmarshalLegacyString(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write old-style projects as plain strings
	content := `projects:
  myapp: /path/to/myapp
  other: /path/to/other
`
	os.WriteFile(configPath, []byte(content), 0644)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if proj, ok := cfg.Projects["myapp"]; !ok {
		t.Error("expected myapp project")
	} else if proj.Path != "/path/to/myapp" {
		t.Errorf("expected path /path/to/myapp, got %q", proj.Path)
	} else if proj.Serve {
		t.Error("expected serve to be false for legacy entries")
	}
}

func TestProjectUnmarshalNewFormat(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	content := `projects:
  myapp:
    path: /path/to/myapp
    serve: true
  other:
    path: /path/to/other
`
	os.WriteFile(configPath, []byte(content), 0644)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	myapp := cfg.Projects["myapp"]
	if myapp.Path != "/path/to/myapp" {
		t.Errorf("path: got %q", myapp.Path)
	}
	if !myapp.Serve {
		t.Error("expected serve=true")
	}

	other := cfg.Projects["other"]
	if other.Serve {
		t.Error("expected serve=false when omitted")
	}
}

func TestNodeReposMigratedToProjects(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Old-style config with node.repos
	content := `node:
  id: testnode
  grpc_port: 50051
  gossip_port: 7946
  repos:
    - name: myapp
      path: /srv/myapp
`
	os.WriteFile(configPath, []byte(content), 0644)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// node.repos should be cleared
	if len(cfg.Node.Repos) != 0 {
		t.Errorf("expected node.repos to be cleared after migration, got %v", cfg.Node.Repos)
	}

	// project should exist with serve: true
	proj, ok := cfg.Projects["myapp"]
	if !ok {
		t.Fatal("expected myapp to be migrated to projects")
	}
	if proj.Path != "/srv/myapp" {
		t.Errorf("path: got %q, want /srv/myapp", proj.Path)
	}
	if !proj.Serve {
		t.Error("expected migrated project to have serve: true")
	}
}

func TestNodeReposMigrationPreservesExistingProject(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Project already exists in new format; node.repos also references it
	content := `projects:
  myapp:
    path: /srv/myapp
node:
  repos:
    - name: myapp
      path: /srv/myapp
`
	os.WriteFile(configPath, []byte(content), 0644)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(cfg.Node.Repos) != 0 {
		t.Error("expected node.repos cleared")
	}
	if !cfg.Projects["myapp"].Serve {
		t.Error("expected existing project to get serve: true from migration")
	}
}

func TestEnsureNodeDefaults_GeneratesID(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Node.ID = "" // simulate missing

	changes := cfg.EnsureNodeDefaults()

	if cfg.Node.ID == "" {
		t.Error("expected node ID to be set from hostname")
	}
	if len(changes) == 0 {
		t.Error("expected at least one change to be reported")
	}
	found := false
	for _, c := range changes {
		if len(c) > 8 && c[:8] == "node.id " {
			found = true
		}
	}
	if !found {
		t.Errorf("expected node.id in changes, got %v", changes)
	}
}

func TestEnsureNodeDefaults_NoChangesWhenFullyConfigured(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Node.ID = "my-node"
	cfg.Node.GRPCPort = 50051
	cfg.Node.GossipPort = 7946
	cfg.Node.Auth.Mode = "none"
	cfg.Node.Sync.MaxFileSizeBytes = 50 * 1024 * 1024

	changes := cfg.EnsureNodeDefaults()
	if len(changes) != 0 {
		t.Errorf("expected no changes for fully configured node, got %v", changes)
	}
}

func TestEnsureNodeDefaults_FillsZeroPorts(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Node.ID = "test-node"
	cfg.Node.GRPCPort = 0
	cfg.Node.GossipPort = 0

	changes := cfg.EnsureNodeDefaults()

	if cfg.Node.GRPCPort != 50051 {
		t.Errorf("expected GRPCPort 50051, got %d", cfg.Node.GRPCPort)
	}
	if cfg.Node.GossipPort != 7946 {
		t.Errorf("expected GossipPort 7946, got %d", cfg.Node.GossipPort)
	}
	if len(changes) != 2 {
		t.Errorf("expected 2 changes, got %d: %v", len(changes), changes)
	}
}

func TestEnsureNodeDefaults_FillsAuthAndSync(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Node.ID = "test-node"
	cfg.Node.Auth.Mode = ""
	cfg.Node.Sync.MaxFileSizeBytes = 0

	changes := cfg.EnsureNodeDefaults()

	if cfg.Node.Auth.Mode != "none" {
		t.Errorf("expected auth mode %q, got %q", "none", cfg.Node.Auth.Mode)
	}
	if cfg.Node.Sync.MaxFileSizeBytes != 50*1024*1024 {
		t.Errorf("expected max file size 50 MiB, got %d", cfg.Node.Sync.MaxFileSizeBytes)
	}
	if len(changes) != 2 {
		t.Errorf("expected 2 changes, got %d: %v", len(changes), changes)
	}
}

func TestEnsureNodeDefaults_PersistsViaRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Save a config with no node.id
	original := DefaultConfig()
	original.Node.ID = ""
	if err := original.Save(configPath); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Load it, apply defaults, save again
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	changes := loaded.EnsureNodeDefaults()
	if len(changes) == 0 {
		t.Fatal("expected defaults to be applied")
	}
	if err := loaded.Save(configPath); err != nil {
		t.Fatalf("save after defaults: %v", err)
	}

	// Reload and verify ID is persisted
	reloaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Node.ID == "" {
		t.Error("expected node ID to survive save/load round-trip")
	}
}

func TestConfigPathsExpansion(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-config-expand-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `state_dir: "~/phantom-test"
paths:
  overlays: "~/phantom-test/overlays"
  mounts: "~/phantom-test/mounts"
  logs: "~/phantom-test/logs"
  snapshots: "~/phantom-test/snapshots"
logging:
  level: info
  file: "~/phantom-test/phantom.log"
`
	os.WriteFile(configPath, []byte(content), 0644)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	homeDir, _ := os.UserHomeDir()
	expected := filepath.Join(homeDir, "phantom-test")
	if cfg.StateDir != expected {
		t.Errorf("StateDir = %q, want %q", cfg.StateDir, expected)
	}
}
