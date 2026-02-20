package commands

import (
	"path/filepath"
	"testing"

	"github.com/martinsuchenak/phantom/internal/config"
	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/martinsuchenak/phantom/pkg/api"
)

// setupTestEnv sets up cfg and log globals for testing, returns cleanup func
func setupTestEnv(t *testing.T, tmpDir string) {
	t.Helper()
	oldCfg := cfg
	oldLog := log
	t.Cleanup(func() {
		cfg = oldCfg
		log = oldLog
	})

	cfg = &config.Config{
		StateDir: filepath.Join(tmpDir, "state"),
		Logging:  config.Logging{Level: "info"},
		Overlay:  config.Overlay{AutoCleanupDays: 7},
		Git: config.Git{
			AutoBranch:   true,
			BranchPrefix: "phantom/",
		},
		Darwin: config.Darwin{
			UnionFSPath: "unionfs-fuse",
			FUSEOptions: []string{"cow"},
		},
		Agent: config.Agent{
			DefaultTimeoutMinutes: 60,
			CleanupOnSuccess:      true,
		},
	}
	log = &MockLogger{}
}

// createTestStore creates a state store in the test tmpDir
func createTestStore(t *testing.T, tmpDir string) *state.Store {
	t.Helper()
	store, err := state.NewStore(filepath.Join(tmpDir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// testOverlay creates a minimal api.Overlay for testing
func testOverlay(name, baseDir, mountPoint, upperDir string) api.Overlay {
	return api.Overlay{
		Name:       name,
		BaseDir:    baseDir,
		MountPoint: mountPoint,
		UpperDir:   upperDir,
	}
}
