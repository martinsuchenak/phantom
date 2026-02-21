package commands

import (
	"context"
	"testing"
	"time"

	"github.com/martinsuchenak/phantom/pkg/api"
)

func TestNewPinCommand(t *testing.T) {
	cmd := NewPinCommand()
	if cmd.Name != "pin" {
		t.Errorf("expected command name 'pin', got %q", cmd.Name)
	}
}

func TestNewUnpinCommand(t *testing.T) {
	cmd := NewUnpinCommand()
	if cmd.Name != "unpin" {
		t.Errorf("expected command name 'unpin', got %q", cmd.Name)
	}
}

func TestCheckPinDivergence(t *testing.T) {
	t.Run("not pinned", func(t *testing.T) {
		ovl := &api.Overlay{Name: "test", BaseDir: "/tmp"}
		diverged, _, err := CheckPinDivergence(context.Background(), ovl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if diverged {
			t.Error("expected no divergence for unpinned overlay")
		}
	})

	t.Run("pinned but not git repo", func(t *testing.T) {
		tmpDir := t.TempDir()
		ovl := &api.Overlay{
			Name:         "test",
			BaseDir:      tmpDir,
			PinnedCommit: "abc123",
		}
		diverged, _, err := CheckPinDivergence(context.Background(), ovl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if diverged {
			t.Error("expected no divergence for non-git base")
		}
	})
}

func TestSetLockAndPin(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	store := createTestStore(t, tmpDir)
	ovl := &api.Overlay{
		Name:         "test-pin",
		BaseDir:      tmpDir,
		UpperDir:     tmpDir,
		PinnedCommit: "abc123def456",
		Locked:       true,
		CreatedAt:    time.Now(),
	}
	if err := store.Save(ovl); err != nil {
		t.Fatal(err)
	}

	// Verify fields persist through save/load
	loaded, err := store.Load("test-pin")
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Locked {
		t.Error("expected Locked to persist")
	}
	if loaded.PinnedCommit != "abc123def456" {
		t.Errorf("expected PinnedCommit to persist, got %q", loaded.PinnedCommit)
	}
}
