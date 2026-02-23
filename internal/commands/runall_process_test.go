package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/martinsuchenak/phantom/internal/config"
)

func TestLoadAgentsConfig_Valid(t *testing.T) {
	tmpDir := t.TempDir()

	configContent := `agents:
  - name: agent-1
    agent: "claude --print"
    task: "fix bugs"
    branch: "feature/fix"
  - name: agent-2
    agent: "aider --message test"
`
	configPath := filepath.Join(tmpDir, "agents.yaml")
	os.WriteFile(configPath, []byte(configContent), 0600)

	agents, err := loadAgentsConfig(configPath)
	if err != nil {
		t.Fatalf("loadAgentsConfig failed: %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}
	if agents[0].Name != "agent-1" {
		t.Errorf("expected agent-1, got %s", agents[0].Name)
	}
	if agents[0].Agent != "claude --print" {
		t.Errorf("expected 'claude --print', got %s", agents[0].Agent)
	}
	if agents[0].Task != "fix bugs" {
		t.Errorf("expected 'fix bugs', got %s", agents[0].Task)
	}
}

func TestLoadAgentsConfig_AutoName(t *testing.T) {
	tmpDir := t.TempDir()

	configContent := `agents:
  - agent: "claude --print"
`
	configPath := filepath.Join(tmpDir, "agents.yaml")
	os.WriteFile(configPath, []byte(configContent), 0600)

	agents, err := loadAgentsConfig(configPath)
	if err != nil {
		t.Fatalf("loadAgentsConfig failed: %v", err)
	}
	if agents[0].Name != "agent-1" {
		t.Errorf("expected auto-generated name 'agent-1', got %s", agents[0].Name)
	}
}

func TestLoadAgentsConfig_WithTimeout(t *testing.T) {
	tmpDir := t.TempDir()

	configContent := `agents:
  - name: timed
    agent: "claude --print"
    timeout: 120
`
	configPath := filepath.Join(tmpDir, "agents.yaml")
	os.WriteFile(configPath, []byte(configContent), 0600)

	agents, err := loadAgentsConfig(configPath)
	if err != nil {
		t.Fatalf("loadAgentsConfig failed: %v", err)
	}
	if agents[0].Timeout != 120 {
		t.Errorf("expected timeout 120, got %d", agents[0].Timeout)
	}
}

func TestGetConfigMode_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "bad.yaml")
	os.WriteFile(configPath, []byte("{{invalid yaml"), 0600)

	mode, _, _ := getConfigMode(configPath)
	if mode != "parallel" {
		t.Errorf("expected parallel default for invalid yaml, got %s", mode)
	}
}

func TestProcessRunAll(t *testing.T) {
	mockBinDir, cleanup := setupMockPath(t)
	defer cleanup()

	tmpDir := t.TempDir()

	oldCfg := cfg
	oldLog := log
	defer func() {
		cfg = oldCfg
		log = oldLog
	}()

	cfg = &config.Config{
		StateDir: filepath.Join(tmpDir, "state"),
		Darwin: config.Darwin{
			UnionFSPath: "unionfs-fuse",
		},
		Agent: config.Agent{
			DefaultTimeoutMinutes: 1,
		},
	}
	log = &MockLogger{}

	baseDir := filepath.Join(tmpDir, "base")
	os.MkdirAll(baseDir, 0755)

	mountPoint := filepath.Join(tmpDir, "state", "mnt", "agent-1")
	updateMockMount(t, mockBinDir, mountPoint)

	agents := []agentDef{
		{
			Name:  "agent-1",
			Agent: "echo hello",
		},
	}

	err := processRunAll(context.Background(), baseDir, agents, 1, true, false, "json", false)
	if err != nil {
		t.Fatalf("processRunAll failed: %v", err)
	}
}
