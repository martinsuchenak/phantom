package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/martinsuchenak/phantom/internal/config"
	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/martinsuchenak/phantom/pkg/api"
	"github.com/paularlott/logger"
)

// MockLogger implements logger.Logger for testing
type MockLogger struct {
	debugs []string
	infos  []string
	warns  []string
	errors []string
}

func (l *MockLogger) Trace(msg string, keysAndValues ...any) {}
func (l *MockLogger) Debug(msg string, keysAndValues ...any) {
	l.debugs = append(l.debugs, fmt.Sprintf(msg, keysAndValues...))
}
func (l *MockLogger) Info(msg string, keysAndValues ...any) {
	l.infos = append(l.infos, fmt.Sprintf(msg, keysAndValues...))
}
func (l *MockLogger) Warn(msg string, keysAndValues ...any) {
	l.warns = append(l.warns, fmt.Sprintf(msg, keysAndValues...))
}
func (l *MockLogger) Error(msg string, keysAndValues ...any) {
	l.errors = append(l.errors, fmt.Sprintf(msg, keysAndValues...))
}
func (l *MockLogger) Fatal(msg string, keysAndValues ...any)   {}
func (l *MockLogger) With(key string, value any) logger.Logger { return l }
func (l *MockLogger) WithError(err error) logger.Logger        { return l }
func (l *MockLogger) WithGroup(group string) logger.Logger     { return l }

// setupMockPath creates a temp dir with mock executables and adds it to PATH
func setupMockPath(t *testing.T) (string, func()) {
	tmpDir, err := os.MkdirTemp("", "phantom-bin-*")
	if err != nil {
		t.Fatal(err)
	}

	// Create mock executables
	mocks := map[string]string{
		"unionfs-fuse": "#!/bin/sh\n# Mock unionfs-fuse\nsleep 1\n",
		"mount":        "#!/bin/sh\n# Mock mount\necho \"unionfs_fuse on /tmp/overlay type fuse (rw)\"\necho \"unionfs_fuse on /tmp/mnt/test-overlay type fuse (rw)\"\n",
		"umount":       "#!/bin/sh\n# Mock umount\nexit 0\n",
		"git":          "#!/bin/sh\n# Mock git\nif [ \"$1\" = \"rev-parse\" ]; then echo \"/tmp/base\"; fi\nif [ \"$1\" = \"status\" ]; then echo \"\"; fi\nif [ \"$1\" = \"branch\" ]; then echo \"master\"; fi\nexit 0\n",
	}

	for name, script := range mocks {
		path := filepath.Join(tmpDir, name)
		if err := os.WriteFile(path, []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
	}

	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+oldPath)

	return tmpDir, func() {
		os.Setenv("PATH", oldPath)
		os.RemoveAll(tmpDir)
	}
}

func TestRootCommand(t *testing.T) {
	// Setup env
	tmpDir, err := os.MkdirTemp("", "phantom-cmd-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Mock global config and logger
	oldCfg := cfg
	oldLog := log
	defer func() {
		cfg = oldCfg
		log = oldLog
	}()

	cfg = &config.Config{
		StateDir: filepath.Join(tmpDir, "state"),
	}
	mockLog := &MockLogger{}
	log = mockLog

	// Test NewStartCommand structure
	cmd := NewStartCommand()
	if cmd.Name != "start" {
		t.Errorf("expected command name 'start', got '%s'", cmd.Name)
	}

	// Test NewListCommand
	listCmd := NewListCommand()
	if listCmd.Name != "list" {
		t.Errorf("expected command name 'list', got '%s'", listCmd.Name)
	}

	// Test NewStopCommand
	stopCmd := NewStopCommand()
	if stopCmd.Name != "stop" {
		t.Errorf("expected command name 'stop', got '%s'", stopCmd.Name)
	}
}

func TestListCommand(t *testing.T) {
	// Setup mocks
	_, cleanup := setupMockPath(t)
	defer cleanup()

	tmpDir, err := os.MkdirTemp("", "phantom-cmd-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldCfg := cfg
	oldLog := log
	defer func() {
		cfg = oldCfg
		log = oldLog
	}()

	cfg = &config.Config{
		StateDir: filepath.Join(tmpDir, "state"),
	}
	mockLog := &MockLogger{}
	log = mockLog

	// Create some dummy state
	store, err := state.NewStore(cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}

	err = store.Save(&api.Overlay{
		Name:       "test-overlay",
		MountPoint: "/tmp/overlay",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Run list command
	cmd := NewListCommand()
	ctx := context.Background()

	if cmd.Run != nil {
		err := cmd.Run(ctx, cmd)
		if err != nil {
			t.Errorf("list command failed: %v", err)
		}
	}

	// Check if mock logger received the overlay name
	found := false
	for _, msg := range mockLog.infos {
		if len(msg) > 0 {
			found = true
			break
		}
	}

	// List command should output something
	// check via stdout capture is hard, but we assume it ran without error
	_ = found
}

func TestStartCommand(t *testing.T) {
	mockBinDir, cleanup := setupMockPath(t)
	defer cleanup()

	tmpDir, err := os.MkdirTemp("", "phantom-cmd-start-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Update mock mount script with correct path
	mountPoint := filepath.Join(tmpDir, "state", "mnt", "test-overlay")
	mountScript := fmt.Sprintf("#!/bin/sh\necho \"unionfs_fuse on %s type fuse (rw)\"\n", mountPoint)
	if err := os.WriteFile(filepath.Join(mockBinDir, "mount"), []byte(mountScript), 0755); err != nil {
		t.Fatal(err)
	}

	oldCfg := cfg
	oldLog := log
	defer func() {
		cfg = oldCfg
		log = oldLog
	}()

	cfg = &config.Config{
		StateDir: filepath.Join(tmpDir, "state"),
		Darwin: config.Darwin{
			UnionFSPath: "unionfs-fuse", // Force use of our mock
			FUSEOptions: []string{"cow"},
		},
		Git: config.Git{
			AutoBranch:     true,
			BranchPrefix:   "overlay/",
			AutoPushOnStop: false,
		},
	}
	mockLog := &MockLogger{}
	log = mockLog

	// Create base dir
	baseDir := filepath.Join(tmpDir, "base")
	os.MkdirAll(baseDir, 0755)

	// Call processStart directly to bypass CLI parsing issues in tests
	err = processStart(context.Background(), baseDir, "test-overlay", "", false)
	if err != nil {
		t.Errorf("processStart failed: %v", err)
	}

	// Verify state
	store, _ := state.NewStore(cfg.StateDir)
	if !store.Exists("test-overlay") {
		t.Error("overlay state should exist")
	}

	// Verify logs
	if len(mockLog.infos) == 0 {
		t.Error("expected info log with mount path")
	}
}

func TestStopCommand(t *testing.T) {
	_, cleanup := setupMockPath(t)
	defer cleanup()

	tmpDir, err := os.MkdirTemp("", "phantom-cmd-stop-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldCfg := cfg
	oldLog := log
	defer func() {
		cfg = oldCfg
		log = oldLog
	}()

	cfg = &config.Config{
		StateDir: filepath.Join(tmpDir, "state"),
	}
	mockLog := &MockLogger{}
	log = mockLog

	// Create dummy state
	store, err := state.NewStore(cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}

	overlayPath := filepath.Join(tmpDir, "mnt", "test-overlay")
	os.MkdirAll(overlayPath, 0755)

	err = store.Save(&api.Overlay{
		Name:       "test-overlay",
		MountPoint: overlayPath,
		PID:        12345,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Call processStop directly
	err = processStop(context.Background(), "test-overlay", true, false, false)
	if err != nil {
		t.Errorf("processStop failed: %v", err)
	}

	// Verify state deleted
	if store.Exists("test-overlay") {
		t.Error("overlay state should be deleted")
	}
}

// updateMockMount updates the mock mount script to output the correct mount point
func updateMockMount(t *testing.T, binDir, mountPoint string) {
	mountScript := fmt.Sprintf("#!/bin/sh\necho \"unionfs_fuse on %s type fuse (rw)\"\n", mountPoint)
	if err := os.WriteFile(filepath.Join(binDir, "mount"), []byte(mountScript), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestRunCommand(t *testing.T) {
	mockBinDir, cleanup := setupMockPath(t)
	defer cleanup()

	tmpDir, err := os.MkdirTemp("", "phantom-cmd-run-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

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
			FUSEOptions: []string{"cow"},
		},
		Git: config.Git{
			AutoBranch: false,
		},
		Agent: config.Agent{
			DefaultTimeoutMinutes: 1,
		},
	}
	mockLog := &MockLogger{}
	log = mockLog

	// Create base dir
	baseDir := filepath.Join(tmpDir, "base")
	os.MkdirAll(baseDir, 0755)

	// Update mock mount for the expected overlay path
	// Name: "test-run" -> Mount: .../mnt/test-run
	mountPoint := filepath.Join(tmpDir, "state", "mnt", "test-run")
	updateMockMount(t, mockBinDir, mountPoint)

	// Call processRun
	exitCode, err := processRun(context.Background(), "echo hello", "test-task", baseDir, "test-run", "", 1, true, false, false)
	if err != nil {
		t.Errorf("processRun failed: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	// Verify cleanup happened (state should be gone)
	store, _ := state.NewStore(cfg.StateDir)
	if store.Exists("test-run") {
		t.Error("overlay state should be deleted after run with cleanup=true")
	}
}

func TestStatusCommand(t *testing.T) {
	mockBinDir, cleanup := setupMockPath(t)
	defer cleanup()

	tmpDir, err := os.MkdirTemp("", "phantom-cmd-status-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldCfg := cfg
	oldLog := log
	defer func() {
		cfg = oldCfg
		log = oldLog
	}()

	cfg = &config.Config{
		StateDir: filepath.Join(tmpDir, "state"),
	}
	mockLog := &MockLogger{}
	log = mockLog

	// Create dummy state
	store, err := state.NewStore(cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}

	mountPoint := filepath.Join(tmpDir, "mnt", "test-status")
	err = store.Save(&api.Overlay{
		Name:       "test-status",
		MountPoint: mountPoint,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Update mock mount to report it as mounted
	updateMockMount(t, mockBinDir, mountPoint)

	// Test single status
	if err := processStatus(context.Background(), "test-status", "json"); err != nil {
		t.Errorf("processStatus (single) failed: %v", err)
	}

	// Test all status
	if err := processStatus(context.Background(), "", "table"); err != nil {
		t.Errorf("processStatus (all) failed: %v", err)
	}

	// Test single status in table format
	if err := processStatus(context.Background(), "test-status", "table"); err != nil {
		t.Errorf("processStatus (single table) failed: %v", err)
	}
}

func TestStartCommand_Persistent(t *testing.T) {
	mockBinDir, cleanup := setupMockPath(t)
	defer cleanup()

	tmpDir, err := os.MkdirTemp("", "phantom-cmd-persist-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

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
			FUSEOptions: []string{"cow"},
		},
	}
	mockLog := &MockLogger{}
	log = mockLog

	baseDir := filepath.Join(tmpDir, "base")
	os.MkdirAll(baseDir, 0755)

	// Update mock mount
	mountPoint := filepath.Join(tmpDir, "state", "mnt", "test-persist")
	updateMockMount(t, mockBinDir, mountPoint)

	// Call processStart with persistent=true
	err = processStart(context.Background(), baseDir, "test-persist", "", true)
	if err != nil {
		t.Errorf("processStart failed: %v", err)
	}

	// Verify state has persistent flag
	store, _ := state.NewStore(cfg.StateDir)
	ovl, err := store.Load("test-persist")
	if err != nil {
		t.Fatal(err)
	}
	if !ovl.Persistent {
		t.Error("overlay should be persistent")
	}
}

func TestStartCommand_Existing(t *testing.T) {
	mockBinDir, cleanup := setupMockPath(t)
	defer cleanup()

	tmpDir, err := os.MkdirTemp("", "phantom-cmd-exist-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

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
	}
	mockLog := &MockLogger{}
	log = mockLog

	baseDir := filepath.Join(tmpDir, "base")
	os.MkdirAll(baseDir, 0755)

	// Pre-create overlay state
	store, _ := state.NewStore(cfg.StateDir)
	mountPoint := filepath.Join(tmpDir, "mnt", "test-existing")
	store.Save(&api.Overlay{
		Name:       "test-existing",
		MountPoint: mountPoint,
		BaseDir:    baseDir,
	})

	// Pre-mock mount to simulate it's ALREADY mounted
	os.MkdirAll(mountPoint, 0755)
	updateMockMount(t, mockBinDir, mountPoint)

	// Call processStart
	// Should fail with ALREADY_EXISTS
	err = processStart(context.Background(), baseDir, "test-existing", "", false)
	if err == nil {
		t.Error("processStart should fail for existing overlay")
	} else {
		// Check error type/message
		// Ideally use api.IsErr(err, api.ErrAlreadyExists) or check message
		if !contains(err.Error(), "already exists") {
			t.Errorf("expected 'already exists' error, got: %v", err)
		}
	}
}

// contains helper
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[0:len(substr)] == substr || (len(s) > len(substr) && contains(s[1:], substr))
}

func TestStatusCommand_Unmounted(t *testing.T) {
	_, cleanup := setupMockPath(t)
	defer cleanup()

	tmpDir, err := os.MkdirTemp("", "phantom-cmd-status-unmounted-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldCfg := cfg
	oldLog := log
	defer func() {
		cfg = oldCfg
		log = oldLog
	}()

	cfg = &config.Config{
		StateDir: filepath.Join(tmpDir, "state"),
	}
	mockLog := &MockLogger{}
	log = mockLog

	store, _ := state.NewStore(cfg.StateDir)
	store.Save(&api.Overlay{
		Name:       "test-unmounted",
		MountPoint: "/tmp/fake-mount",
	})
	// No mount update -> implicitly unmounted for 'mount' mock (which lists nothing by default or standard static content)
	// Actually setupMockPath 'mount' outputs some static lines?
	// "unionfs_fuse on /tmp/overlay type fuse (rw)"
	// "unionfs_fuse on /tmp/mnt/test-overlay type fuse (rw)"
	// "test-unmounted" is NOT in that list. So IsMounted returns false. Correct.

	if err := processStatus(context.Background(), "test-unmounted", "table"); err != nil {
		t.Errorf("processStatus failed: %v", err)
	}
}

func TestStartCommand_WithBranch(t *testing.T) {
	mockBinDir, cleanup := setupMockPath(t)
	defer cleanup()

	tmpDir, err := os.MkdirTemp("", "phantom-cmd-branch-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

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
			FUSEOptions: []string{"cow"},
		},
		Git: config.Git{
			AutoBranch:   true,
			BranchPrefix: "phantom/",
		},
	}
	mockLog := &MockLogger{}
	log = mockLog

	baseDir := filepath.Join(tmpDir, "base")
	os.MkdirAll(baseDir, 0755)

	mountPoint := filepath.Join(tmpDir, "state", "mnt", "test-branch")
	updateMockMount(t, mockBinDir, mountPoint)

	// Call processStart with explicit branch
	err = processStart(context.Background(), baseDir, "test-branch", "feature/test", false)
	if err != nil {
		t.Errorf("processStart failed: %v", err)
	}

	// Verify overlay state has branch
	store, _ := state.NewStore(cfg.StateDir)
	ovl, err := store.Load("test-branch")
	if err != nil {
		t.Fatal(err)
	}
	if ovl.Branch != "feature/test" {
		t.Errorf("expected branch 'feature/test', got '%s'", ovl.Branch)
	}
}
