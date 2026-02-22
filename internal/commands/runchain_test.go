package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/martinsuchenak/phantom/internal/config"
)

func TestNewRunChainCommand(t *testing.T) {
	cmd := NewRunChainCommand()
	if cmd.Name != "run-chain" {
		t.Errorf("expected command name 'run-chain', got %q", cmd.Name)
	}
}

func TestLoadChainConfig(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("valid config", func(t *testing.T) {
		yaml := `name: my-chain
branch: feature/chain
steps:
  - name: step-1
    agent: "echo hello"
    task: "do something"
    timeout: 30
  - agent: "echo world"
`
		path := tmpDir + "/chain.yaml"
		os.WriteFile(path, []byte(yaml), 0644)

		cc, err := loadChainConfig(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cc.Name != "my-chain" {
			t.Errorf("expected name 'my-chain', got %q", cc.Name)
		}
		if cc.Branch != "feature/chain" {
			t.Errorf("expected branch 'feature/chain', got %q", cc.Branch)
		}
		if len(cc.Steps) != 2 {
			t.Fatalf("expected 2 steps, got %d", len(cc.Steps))
		}
		if cc.Steps[0].Name != "step-1" {
			t.Errorf("expected step name 'step-1', got %q", cc.Steps[0].Name)
		}
		if cc.Steps[1].Name != "step-2" {
			t.Errorf("expected auto-name 'step-2', got %q", cc.Steps[1].Name)
		}
	})

	t.Run("missing agent", func(t *testing.T) {
		yaml := `steps:
  - name: bad
    task: "no agent"
`
		path := tmpDir + "/bad-chain.yaml"
		os.WriteFile(path, []byte(yaml), 0644)

		_, err := loadChainConfig(path)
		if err == nil {
			t.Error("expected error for missing agent")
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := loadChainConfig("/nonexistent/chain.yaml")
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		path := tmpDir + "/invalid.yaml"
		os.WriteFile(path, []byte("{{invalid"), 0644)

		_, err := loadChainConfig(path)
		if err == nil {
			t.Error("expected error for invalid yaml")
		}
	})
}

func TestParseInlineSteps(t *testing.T) {
	steps := parseInlineSteps("echo hello,echo world,echo done")
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}
	if steps[0].Agent != "echo hello" {
		t.Errorf("expected agent 'echo hello', got %q", steps[0].Agent)
	}
	if steps[2].Agent != "echo done" {
		t.Errorf("expected agent 'echo done', got %q", steps[2].Agent)
	}
}

func TestParseInlineStepsEmpty(t *testing.T) {
	steps := parseInlineSteps("")
	if len(steps) != 0 {
		t.Errorf("expected 0 steps for empty input, got %d", len(steps))
	}
}

func TestPrintChainSummary_Table(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	results := []agentResult{
		{Name: "step-1", Agent: "echo hello", ExitCode: 0, Duration: 3 * time.Second},
		{Name: "step-2", Agent: "echo world", ExitCode: 0, Duration: 5 * time.Second},
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printChainSummary(results, "table", false)

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "step-1") {
		t.Errorf("expected 'step-1' in output, got %q", output)
	}
}

func TestPrintChainSummary_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	results := []agentResult{
		{Name: "step-1", Agent: "echo hello", ExitCode: 0, Duration: 3 * time.Second},
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printChainSummary(results, "json", false)

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.Contains(output, `"name": "step-1"`) {
		t.Errorf("expected JSON with step-1, got %q", output)
	}
}

func TestPrintChainSummary_StoppedEarly(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	results := []agentResult{
		{Name: "step-1", Agent: "echo hello", ExitCode: 1, Duration: 1 * time.Second},
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printChainSummary(results, "table", true)

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// Table output should contain the step name and FAILED status
	if !strings.Contains(output, "FAILED") {
		t.Errorf("expected 'FAILED' in output, got %q", output)
	}
	if !strings.Contains(output, "step-1") {
		t.Errorf("expected 'step-1' in output, got %q", output)
	}
}

func TestProcessRunChain(t *testing.T) {
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

	name := "chain-test"
	mountPoint := filepath.Join(tmpDir, "state", "mnt", name)
	updateMockMount(t, mockBinDir, mountPoint)

	steps := []chainStep{
		{
			Name:  "step-1",
			Agent: "echo step1",
		},
		{
			Name:  "step-2",
			Agent: "echo step2", // Mock agent running a failing command
		},
	}

	err := processRunChain(context.Background(), baseDir, name, "", steps, 1, true, false, true, "json")
	if err != nil {
		t.Fatalf("processRunChain failed: %v", err)
	}
}
