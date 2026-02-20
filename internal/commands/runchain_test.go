package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewRunChainCommand(t *testing.T) {
	cmd := NewRunChainCommand()
	if cmd.Name != "run-chain" {
		t.Errorf("expected command name 'run-chain', got %q", cmd.Name)
	}
	if len(cmd.Flags) == 0 {
		t.Error("expected flags to be defined")
	}
	if len(cmd.Arguments) == 0 {
		t.Error("expected arguments to be defined")
	}
}

func TestLoadChainConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		content := `name: my-chain
branch: feature/chain
steps:
  - name: implement
    agent: "claude --print"
    task: "implement feature"
    timeout: 30
  - name: test
    agent: "aider --message \"{task}\""
    task: "write tests"
`
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "chain.yaml")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}

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
		if cc.Steps[0].Name != "implement" {
			t.Errorf("expected step name 'implement', got %q", cc.Steps[0].Name)
		}
		if cc.Steps[0].Timeout != 30 {
			t.Errorf("expected timeout 30, got %d", cc.Steps[0].Timeout)
		}
	})

	t.Run("auto-names steps", func(t *testing.T) {
		content := `steps:
  - agent: "claude --print"
  - agent: "aider"
`
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "chain.yaml")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}

		cc, err := loadChainConfig(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cc.Steps[0].Name != "step-1" {
			t.Errorf("expected auto-name 'step-1', got %q", cc.Steps[0].Name)
		}
		if cc.Steps[1].Name != "step-2" {
			t.Errorf("expected auto-name 'step-2', got %q", cc.Steps[1].Name)
		}
	})

	t.Run("missing agent", func(t *testing.T) {
		content := `steps:
  - name: bad-step
    task: "no agent"
`
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "chain.yaml")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}

		_, err := loadChainConfig(path)
		if err == nil {
			t.Error("expected error for missing agent")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := loadChainConfig("/nonexistent/chain.yaml")
		if err == nil {
			t.Error("expected error for missing file")
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "bad.yaml")
		if err := os.WriteFile(path, []byte("{{invalid"), 0600); err != nil {
			t.Fatal(err)
		}

		_, err := loadChainConfig(path)
		if err == nil {
			t.Error("expected error for invalid YAML")
		}
	})
}

func TestParseInlineSteps(t *testing.T) {
	steps := parseInlineSteps("claude --print,aider,gemini")
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}
	if steps[0].Agent != "claude --print" {
		t.Errorf("expected 'claude --print', got %q", steps[0].Agent)
	}
	if steps[0].Name != "agent-1" {
		t.Errorf("expected name 'agent-1', got %q", steps[0].Name)
	}
	if steps[2].Agent != "gemini" {
		t.Errorf("expected 'gemini', got %q", steps[2].Agent)
	}
}

func TestParseInlineStepsEmpty(t *testing.T) {
	steps := parseInlineSteps("")
	if len(steps) != 0 {
		t.Errorf("expected 0 steps for empty input, got %d", len(steps))
	}
}

func TestPrintChainSummary(t *testing.T) {
	setupTestEnv(t, t.TempDir())

	results := []agentResult{
		{Name: "step-1", Agent: "claude", ExitCode: 0, Duration: 30 * 1000000000},
		{Name: "step-2", Agent: "aider", ExitCode: 1, Duration: 10 * 1000000000, Error: "failed"},
	}

	t.Run("table format", func(t *testing.T) {
		err := printChainSummary(results, "table", false)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("table format stopped early", func(t *testing.T) {
		err := printChainSummary(results, "table", true)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("json format", func(t *testing.T) {
		err := printChainSummary(results, "json", false)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("all success", func(t *testing.T) {
		successResults := []agentResult{
			{Name: "step-1", Agent: "claude", ExitCode: 0, Duration: 5 * 1000000000},
		}
		err := printChainSummary(successResults, "table", false)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestAgentsToChainSteps(t *testing.T) {
	agents := []agentDef{
		{Name: "a1", Agent: "claude", Task: "do stuff", Timeout: 10},
		{Name: "a2", Agent: "aider", Task: "more stuff", Timeout: 0},
	}

	steps := agentsToChainSteps(agents)
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	if steps[0].Name != "a1" || steps[0].Agent != "claude" || steps[0].Task != "do stuff" || steps[0].Timeout != 10 {
		t.Errorf("step 0 mismatch: %+v", steps[0])
	}
	if steps[1].Name != "a2" || steps[1].Agent != "aider" {
		t.Errorf("step 1 mismatch: %+v", steps[1])
	}
}

func TestGetConfigMode(t *testing.T) {
	t.Run("sequential mode", func(t *testing.T) {
		content := `mode: sequential
name: my-chain
branch: feature/chain
agents:
  - name: a1
    agent: claude
`
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "agents.yaml")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}

		mode, name, branch := getConfigMode(path)
		if mode != "sequential" {
			t.Errorf("expected mode 'sequential', got %q", mode)
		}
		if name != "my-chain" {
			t.Errorf("expected name 'my-chain', got %q", name)
		}
		if branch != "feature/chain" {
			t.Errorf("expected branch 'feature/chain', got %q", branch)
		}
	})

	t.Run("default parallel mode", func(t *testing.T) {
		content := `agents:
  - name: a1
    agent: claude
`
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "agents.yaml")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}

		mode, _, _ := getConfigMode(path)
		if mode != "parallel" {
			t.Errorf("expected mode 'parallel', got %q", mode)
		}
	})

	t.Run("file not found", func(t *testing.T) {
		mode, _, _ := getConfigMode("/nonexistent/file.yaml")
		if mode != "parallel" {
			t.Errorf("expected mode 'parallel' for missing file, got %q", mode)
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "bad.yaml")
		if err := os.WriteFile(path, []byte("{{invalid"), 0600); err != nil {
			t.Fatal(err)
		}

		mode, _, _ := getConfigMode(path)
		if mode != "parallel" {
			t.Errorf("expected mode 'parallel' for invalid yaml, got %q", mode)
		}
	})
}
