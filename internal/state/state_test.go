package state

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/martinsuchenak/phantom/pkg/api"
)

func TestStore(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "phantom-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	t.Run("save and load overlay", func(t *testing.T) {
		now := time.Now()
		overlay := &api.Overlay{
			Name:       "test-overlay",
			BaseDir:    "/path/to/base",
			MountPoint: "/path/to/mnt",
			UpperDir:   "/path/to/upper",
			WorkDir:    "/path/to/work",
			Branch:     "feature/test",
			Persistent: false,
			CreatedAt:  now,
		}

		// Save
		if err := store.Save(overlay); err != nil {
			t.Fatalf("failed to save overlay: %v", err)
		}

		// Load
		loaded, err := store.Load("test-overlay")
		if err != nil {
			t.Fatalf("failed to load overlay: %v", err)
		}

		// Verify
		if loaded.Name != overlay.Name {
			t.Errorf("expected Name %s, got %s", overlay.Name, loaded.Name)
		}
		if loaded.BaseDir != overlay.BaseDir {
			t.Errorf("expected BaseDir %s, got %s", overlay.BaseDir, loaded.BaseDir)
		}
		if loaded.Branch != overlay.Branch {
			t.Errorf("expected Branch %s, got %s", overlay.Branch, loaded.Branch)
		}
		if loaded.Persistent != overlay.Persistent {
			t.Errorf("expected Persistent %v, got %v", overlay.Persistent, loaded.Persistent)
		}
	})

	t.Run("load non-existent overlay", func(t *testing.T) {
		_, err := store.Load("non-existent")
		if err == nil {
			t.Error("expected error for non-existent overlay")
		}

		var overlayErr *api.OverlayError
		if !errors.As(err, &overlayErr) {
			t.Errorf("expected OverlayError, got %T", err)
		} else if overlayErr.Code != api.ErrNotFound {
			t.Errorf("expected error code %s, got %s", api.ErrNotFound, overlayErr.Code)
		}
	})

	t.Run("save overlay with empty name", func(t *testing.T) {
		overlay := &api.Overlay{
			Name: "",
		}

		err := store.Save(overlay)
		if err == nil {
			t.Error("expected error for empty name")
		}
	})

	t.Run("delete overlay", func(t *testing.T) {
		overlay := &api.Overlay{
			Name:       "to-delete",
			BaseDir:    "/path/to/base",
			MountPoint: "/path/to/mnt",
			UpperDir:   "/path/to/upper",
			CreatedAt:  time.Now(),
		}

		// Save
		if err := store.Save(overlay); err != nil {
			t.Fatalf("failed to save overlay: %v", err)
		}

		// Verify exists
		if !store.Exists("to-delete") {
			t.Error("expected overlay to exist")
		}

		// Delete
		if err := store.Delete("to-delete"); err != nil {
			t.Fatalf("failed to delete overlay: %v", err)
		}

		// Verify deleted
		if store.Exists("to-delete") {
			t.Error("expected overlay to be deleted")
		}
	})

	t.Run("load all overlays", func(t *testing.T) {
		// Create multiple overlays
		for i := 0; i < 3; i++ {
			overlay := &api.Overlay{
				Name:       fmt.Sprintf("overlay-%d", i),
				BaseDir:    "/path/to/base",
				MountPoint: "/path/to/mnt",
				UpperDir:   "/path/to/upper",
				CreatedAt:  time.Now(),
			}
			if err := store.Save(overlay); err != nil {
				t.Fatalf("failed to save overlay %d: %v", i, err)
			}
		}

		// Load all
		overlays, err := store.LoadAll()
		if err != nil {
			t.Fatalf("failed to load all overlays: %v", err)
		}

		if len(overlays) < 3 {
			t.Errorf("expected at least 3 overlays, got %d", len(overlays))
		}
	})

	t.Run("exists check", func(t *testing.T) {
		overlay := &api.Overlay{
			Name:       "exists-check",
			BaseDir:    "/path/to/base",
			MountPoint: "/path/to/mnt",
			UpperDir:   "/path/to/upper",
			CreatedAt:  time.Now(),
		}

		if store.Exists("exists-check") {
			t.Error("expected overlay to not exist yet")
		}

		if err := store.Save(overlay); err != nil {
			t.Fatalf("failed to save overlay: %v", err)
		}

		if !store.Exists("exists-check") {
			t.Error("expected overlay to exist")
		}
	})
}

func TestStoreWithPID(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	overlay := &api.Overlay{
		Name:       "with-pid",
		BaseDir:    "/path/to/base",
		MountPoint: "/path/to/mnt",
		UpperDir:   "/path/to/upper",
		CreatedAt:  time.Now(),
		PID:        12345,
	}

	if err := store.Save(overlay); err != nil {
		t.Fatalf("failed to save overlay: %v", err)
	}

	loaded, err := store.Load("with-pid")
	if err != nil {
		t.Fatalf("failed to load overlay: %v", err)
	}

	if loaded.PID != 12345 {
		t.Errorf("expected PID 12345, got %d", loaded.PID)
	}
}

func TestStoreFileFormat(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	overlay := &api.Overlay{
		Name:       "format-test",
		BaseDir:    "/path/to/base",
		MountPoint: "/path/to/mnt",
		UpperDir:   "/path/to/upper",
		Branch:     "test-branch",
		CreatedAt:  time.Now(),
	}

	if err := store.Save(overlay); err != nil {
		t.Fatalf("failed to save overlay: %v", err)
	}

	// Verify file exists and is JSON
	statePath := filepath.Join(tmpDir, "state", "format-test.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("failed to read state file: %v", err)
	}

	// Verify it's valid JSON with expected content
	if !bytes.Contains(data, []byte(`"name": "format-test"`)) {
		t.Error("expected JSON to contain name field")
	}
	if !bytes.Contains(data, []byte(`"branch": "test-branch"`)) {
		t.Error("expected JSON to contain branch field")
	}
}
