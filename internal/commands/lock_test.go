package commands

import (
	"context"
	"testing"
	"time"

	"github.com/martinsuchenak/phantom/pkg/api"
)

func TestNewLockCommand(t *testing.T) {
	cmd := NewLockCommand()
	if cmd.Name != "lock" {
		t.Errorf("expected command name 'lock', got %q", cmd.Name)
	}
}

func TestNewUnlockCommand(t *testing.T) {
	cmd := NewUnlockCommand()
	if cmd.Name != "unlock" {
		t.Errorf("expected command name 'unlock', got %q", cmd.Name)
	}
}

func TestSetLock(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	store := createTestStore(t, tmpDir)
	ovl := &api.Overlay{
		Name:      "test-lock",
		BaseDir:   tmpDir,
		UpperDir:  tmpDir,
		CreatedAt: time.Now(),
	}
	if err := store.Save(ovl); err != nil {
		t.Fatal(err)
	}

	t.Run("lock overlay", func(t *testing.T) {
		if err := setLock("test-lock", true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		loaded, _ := store.Load("test-lock")
		if !loaded.Locked {
			t.Error("expected overlay to be locked")
		}
	})

	t.Run("lock already locked", func(t *testing.T) {
		if err := setLock("test-lock", true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("unlock overlay", func(t *testing.T) {
		if err := setLock("test-lock", false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		loaded, _ := store.Load("test-lock")
		if loaded.Locked {
			t.Error("expected overlay to be unlocked")
		}
	})

	t.Run("unlock already unlocked", func(t *testing.T) {
		if err := setLock("test-lock", false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("lock nonexistent", func(t *testing.T) {
		if err := setLock("nonexistent", true); err == nil {
			t.Error("expected error for nonexistent overlay")
		}
	})
}

func TestDoLock(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	oldLog := log
	log = &MockLogger{}
	defer func() {
		log = oldLog
	}()

	cmd := NewLockCommand()

	runCommandWithArgs(t, []string{"lock"}, func() {
		err := doLock(context.Background(), cmd)
		if err == nil {
			t.Fatalf("expected error for missing name, got none")
		}
	})

	runCommandWithArgs(t, []string{"lock", "nonexistent"}, func() {
		err := doLock(context.Background(), cmd)
		if err == nil {
			t.Fatalf("expected error for nonexistent overlay, got none")
		}
	})
}

func TestDoUnlock(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	oldLog := log
	log = &MockLogger{}
	defer func() {
		log = oldLog
	}()

	cmd := NewUnlockCommand()

	runCommandWithArgs(t, []string{"unlock"}, func() {
		err := doUnlock(context.Background(), cmd)
		if err == nil {
			t.Fatalf("expected error for missing name, got none")
		}
	})

	runCommandWithArgs(t, []string{"unlock", "nonexistent"}, func() {
		err := doUnlock(context.Background(), cmd)
		if err == nil {
			t.Fatalf("expected error for nonexistent overlay, got none")
		}
	})
}
