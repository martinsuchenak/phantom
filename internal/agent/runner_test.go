package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/martinsuchenak/phantom/internal/config"
	"github.com/martinsuchenak/phantom/pkg/api"
	"github.com/paularlott/logger"
)

type mockLogger struct {
	debugs []string
	infos  []string
	errors []string
}

func (m *mockLogger) Trace(msg string, args ...any) {}
func (m *mockLogger) Debug(msg string, args ...any) {
	m.debugs = append(m.debugs, msg)
}
func (m *mockLogger) Info(msg string, args ...any) {
	m.infos = append(m.infos, msg)
}
func (m *mockLogger) Warn(msg string, args ...any) {}
func (m *mockLogger) Error(msg string, args ...any) {
	m.errors = append(m.errors, msg)
}
func (m *mockLogger) Fatal(msg string, args ...any) {}
func (m *mockLogger) With(key string, value any) logger.Logger {
	return m
}
func (m *mockLogger) WithError(err error) logger.Logger {
	return m
}
func (m *mockLogger) WithGroup(group string) logger.Logger {
	return m
}

func TestNewRunner(t *testing.T) {
	cfg := config.DefaultConfig()
	log := &mockLogger{}

	runner := NewRunner(cfg, log)
	if runner == nil {
		t.Fatal("expected runner to not be nil")
	}
	if runner.cfg != cfg {
		t.Error("expected config to be set")
	}
	if runner.gitOps == nil {
		t.Error("expected gitOps to be initialized")
	}
}

func TestBuildEnv(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.AgentEnv = []string{"CUSTOM_VAR=value"}
	log := &mockLogger{}

	runner := NewRunner(cfg, log)

	overlay := &api.Overlay{
		Name:       "test-overlay",
		MountPoint: "/mnt/test",
		BaseDir:    "/base/test",
		Branch:     "test-branch",
	}

	opts := &api.RunOptions{
		Task: "test task",
	}

	env := runner.buildEnv(overlay, opts)

	// Check required env vars are set
	envMap := make(map[string]string)
	for _, e := range env {
		// Parse KEY=VALUE format
		for i, c := range e {
			if c == '=' {
				envMap[e[:i]] = e[i+1:]
				break
			}
		}
	}

	tests := []struct {
		key      string
		expected string
	}{
		{"OVERLAY_NAME", "test-overlay"},
		{"OVERLAY_PATH", "/mnt/test"},
		{"OVERLAY_BASE", "/base/test"},
		{"OVERLAY_BRANCH", "test-branch"},
		{"OVERLAY_TASK", "test task"},
		{"CUSTOM_VAR", "value"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			val, ok := envMap[tt.key]
			if !ok {
				t.Errorf("expected env var %s to be set", tt.key)
				return
			}
			if val != tt.expected {
				t.Errorf("expected %s=%s, got %s", tt.key, tt.expected, val)
			}
		})
	}
}

func TestRunShortCommand(t *testing.T) {
	cfg := config.DefaultConfig()
	log := &mockLogger{} // Use mock logger
	runner := NewRunner(cfg, log)

	// Create temp dir acting as overlay mount
	tmpDir, err := os.MkdirTemp("", "phantom-agent-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	overlay := &api.Overlay{
		Name:       "test-overlay",
		MountPoint: tmpDir,
		BaseDir:    "/base/test",
	}

	opts := &api.RunOptions{
		Agent: "echo 'hello world'",
		Task:  "test task",
	}

	exitCode, err := runner.Run(context.Background(), overlay, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestRunWithTimeout(t *testing.T) {
	cfg := config.DefaultConfig()
	log := &mockLogger{} // Use mock logger
	runner := NewRunner(cfg, log)

	tmpDir, err := os.MkdirTemp("", "phantom-agent-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	overlay := &api.Overlay{
		MountPoint: tmpDir,
	}

	opts := &api.RunOptions{
		Agent:   "sleep 2",
		Task:    "timeout task",
		Timeout: 500 * time.Millisecond,
	}

	// Should fail/timeout
	_, err = runner.Run(context.Background(), overlay, opts)
	if err == nil {
		t.Error("expected error due to timeout")
	}
}

func TestRunExitCodes(t *testing.T) {
	cfg := config.DefaultConfig()
	log := &mockLogger{} // Use mock logger
	runner := NewRunner(cfg, log)

	tmpDir, err := os.MkdirTemp("", "phantom-agent-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	overlay := &api.Overlay{
		MountPoint: tmpDir,
	}

	// Use sh -c to test exit codes since exit is a shell builtin
	opts := &api.RunOptions{
		Agent: "sh -c 'exit 42'",
		Task:  "exit code task",
	}

	exitCode, _ := runner.Run(context.Background(), overlay, opts)
	if exitCode != 42 {
		t.Errorf("expected exit code 42, got %d", exitCode)
	}
}

func TestParseCommandLine(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple command",
			input:    "echo hello",
			expected: []string{"echo", "hello"},
		},
		{
			name:     "command with double quotes",
			input:    `echo "hello world"`,
			expected: []string{"echo", "hello world"},
		},
		{
			name:     "command with single quotes",
			input:    `echo 'hello world'`,
			expected: []string{"echo", "hello world"},
		},
		{
			name:     "mixed quotes",
			input:    `echo "hello" 'world'`,
			expected: []string{"echo", "hello", "world"},
		},
		{
			name:     "multiple spaces",
			input:    "echo    hello    world",
			expected: []string{"echo", "hello", "world"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "only spaces",
			input:    "   ",
			expected: []string{},
		},
		{
			name:     "complex command",
			input:    `python -c "print('hello')"`,
			expected: []string{"python", "-c", "print('hello')"},
		},
		{
			name:     "path with spaces",
			input:    `"/path/to/my program" --arg`,
			expected: []string{"/path/to/my program", "--arg"},
		},
		{
			name:     "tabs as separators",
			input:    "echo\thello\tworld",
			expected: []string{"echo", "hello", "world"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCommandLine(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d args, got %d: %v", len(tt.expected), len(result), result)
				return
			}
			for i, arg := range result {
				if arg != tt.expected[i] {
					t.Errorf("arg %d: expected %q, got %q", i, tt.expected[i], arg)
				}
			}
		})
	}
}

func TestRunWithParsedCommand(t *testing.T) {
	cfg := config.DefaultConfig()
	log := &mockLogger{}
	runner := NewRunner(cfg, log)

	tmpDir, err := os.MkdirTemp("", "phantom-agent-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	overlay := &api.Overlay{
		Name:       "test-overlay",
		MountPoint: tmpDir,
		BaseDir:    "/base/test",
	}

	// Test that command parsing works correctly (no shell injection)
	opts := &api.RunOptions{
		Agent: "echo hello world",
		Task:  "test task",
	}

	exitCode, err := runner.Run(context.Background(), overlay, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestRunHeadless(t *testing.T) {
	cfg := config.DefaultConfig()
	log := &mockLogger{}
	runner := NewRunner(cfg, log)

	tmpDir, err := os.MkdirTemp("", "phantom-agent-headless-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	overlay := &api.Overlay{
		Name:       "headless-test",
		MountPoint: tmpDir,
		BaseDir:    "/base/test",
	}

	opts := &api.RunOptions{
		Agent:    "echo headless",
		Task:     "headless task",
		Headless: true,
	}

	exitCode, err := runner.Run(context.Background(), overlay, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestRunHeadlessWithTaskStdin(t *testing.T) {
	cfg := config.DefaultConfig()
	log := &mockLogger{}
	runner := NewRunner(cfg, log)

	tmpDir, err := os.MkdirTemp("", "phantom-agent-stdin-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	overlay := &api.Overlay{
		Name:       "stdin-test",
		MountPoint: tmpDir,
	}

	// Agent without {task} placeholder + headless + task = stdin pipe
	opts := &api.RunOptions{
		Agent:    "cat",
		Task:     "piped input",
		Headless: true,
	}

	exitCode, err := runner.Run(context.Background(), overlay, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestRunWithTaskPlaceholder(t *testing.T) {
	cfg := config.DefaultConfig()
	log := &mockLogger{}
	runner := NewRunner(cfg, log)

	tmpDir, err := os.MkdirTemp("", "phantom-agent-placeholder-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	overlay := &api.Overlay{
		Name:       "placeholder-test",
		MountPoint: tmpDir,
	}

	opts := &api.RunOptions{
		Agent: `echo "{task}"`,
		Task:  "my task",
	}

	exitCode, err := runner.Run(context.Background(), overlay, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestOpenLogFile(t *testing.T) {
	cfg := config.DefaultConfig()
	tmpDir, err := os.MkdirTemp("", "phantom-agent-logfile-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	cfg.Paths.Logs = filepath.Join(tmpDir, "logs")
	log := &mockLogger{}
	runner := NewRunner(cfg, log)

	f, err := runner.openLogFile("test-overlay")
	if err != nil {
		t.Fatalf("openLogFile failed: %v", err)
	}
	defer func() { _ = f.Close() }()

	// Write something
	_, _ = f.WriteString("test log\n")

	// Verify file exists
	logPath := filepath.Join(tmpDir, "logs", "test-overlay.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("log file not created: %v", err)
	}
	if string(data) != "test log\n" {
		t.Errorf("log content = %q, want %q", string(data), "test log\n")
	}
}

func TestBuildCommand(t *testing.T) {
	cfg := config.DefaultConfig()
	log := &mockLogger{}
	runner := NewRunner(cfg, log)

	// Test normal command
	cmd := runner.buildCommand(context.Background(), "echo hello world")
	if cmd.Path == "" {
		t.Error("expected non-empty command path")
	}

	// Test empty command (fallback)
	cmd2 := runner.buildCommand(context.Background(), "")
	_ = cmd2 // just verify it doesn't panic
}

func TestBuildEnv_NoTask(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.AgentEnv = nil
	log := &mockLogger{}
	runner := NewRunner(cfg, log)

	overlay := &api.Overlay{
		Name:       "test",
		MountPoint: "/mnt",
		BaseDir:    "/base",
	}
	opts := &api.RunOptions{}

	env := runner.buildEnv(overlay, opts)
	found := false
	for _, e := range env {
		if e == "OVERLAY_NAME=test" {
			found = true
		}
	}
	if !found {
		t.Error("expected OVERLAY_NAME in env")
	}
}

func TestHandleGitOperations(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Git.AutoPushOnStop = false
	log := &mockLogger{}
	runner := NewRunner(cfg, log)

	tmpDir, err := os.MkdirTemp("", "phantom-agent-git-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Initialize a git repo
	initGit(t, tmpDir)

	overlay := &api.Overlay{
		Name:       "git-test",
		MountPoint: tmpDir,
		Branch:     "test-branch",
	}

	// No changes — should be a no-op
	runner.handleGitOperations(context.Background(), overlay, true)

	// Create a change
	_ = os.WriteFile(filepath.Join(tmpDir, "new-file.txt"), []byte("change"), 0644)

	// Now there are changes — should commit
	runner.handleGitOperations(context.Background(), overlay, true)
}

func initGit(t *testing.T, dir string) {
	t.Helper()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "init"},
	}
	for _, args := range cmds {
		cmd := exec.CommandContext(context.Background(), args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git command %v failed: %v\n%s", args, err, out)
		}
	}
}
