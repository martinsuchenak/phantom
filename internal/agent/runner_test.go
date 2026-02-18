package agent

import (
	"testing"

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
	// This test requires a valid overlay mount point, so we'll skip it
	// in CI environments or when the overlay can't be created
	t.Skip("requires mounted overlay filesystem")
}

func TestRunWithTimeout(t *testing.T) {
	// This test requires a valid overlay mount point, so we'll skip it
	t.Skip("requires mounted overlay filesystem")
}

func TestRunExitCodes(t *testing.T) {
	// This test requires a valid overlay mount point, so we'll skip it
	t.Skip("requires mounted overlay filesystem")
}
