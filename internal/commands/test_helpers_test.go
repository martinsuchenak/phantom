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

// mockOverlayManager implements overlayManager for testing
type mockOverlayManager struct {
	mounted    map[string]bool
	overlays   map[string]*api.Overlay
	createErr  error
	mountErr   error
	unmountErr error
	cleanupErr error
}

func newMockOverlayManager() *mockOverlayManager {
	return &mockOverlayManager{
		mounted:  make(map[string]bool),
		overlays: make(map[string]*api.Overlay),
	}
}

func (m *mockOverlayManager) Create(opts *api.CreateOptions) (*api.Overlay, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	ovl := &api.Overlay{
		Name:       opts.Name,
		BaseDir:    opts.BaseDir,
		MountPoint: filepath.Join(opts.BaseDir, ".phantom-mnt", opts.Name),
		UpperDir:   filepath.Join(opts.BaseDir, ".phantom-upper", opts.Name),
		Branch:     opts.Branch,
		Persistent: opts.Persistent,
	}
	m.overlays[opts.Name] = ovl
	m.mounted[opts.Name] = true
	return ovl, nil
}

func (m *mockOverlayManager) Mount(overlay *api.Overlay) error {
	if m.mountErr != nil {
		return m.mountErr
	}
	m.mounted[overlay.Name] = true
	return nil
}

func (m *mockOverlayManager) Unmount(overlay *api.Overlay) error {
	if m.unmountErr != nil {
		return m.unmountErr
	}
	m.mounted[overlay.Name] = false
	return nil
}

func (m *mockOverlayManager) IsMounted(overlay *api.Overlay) (bool, error) {
	return m.mounted[overlay.Name], nil
}

func (m *mockOverlayManager) GetStatus(overlay *api.Overlay) (*api.OverlayStatus, error) {
	return &api.OverlayStatus{
		Name:    overlay.Name,
		Mounted: m.mounted[overlay.Name],
	}, nil
}

func (m *mockOverlayManager) Cleanup(overlay *api.Overlay) error {
	if m.cleanupErr != nil {
		return m.cleanupErr
	}
	delete(m.mounted, overlay.Name)
	delete(m.overlays, overlay.Name)
	return nil
}

func (m *mockOverlayManager) Prune() error {
	return nil
}

// setupMockManager sets up a mock overlay manager and returns it + cleanup func
func setupMockManager(t *testing.T) *mockOverlayManager {
	t.Helper()
	mock := newMockOverlayManager()
	oldFunc := createOverlayManagerFunc
	createOverlayManagerFunc = func() (overlayManager, error) {
		return mock, nil
	}
	t.Cleanup(func() {
		createOverlayManagerFunc = oldFunc
	})
	return mock
}
