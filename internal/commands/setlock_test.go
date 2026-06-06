package commands

import (
	"testing"
	"time"

	"github.com/martinsuchenak/phantom/pkg/api"
)

func TestSetLock_Lock(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	store := createTestStore(t, tmpDir)

	ovl := &api.Overlay{
		Name:      "lock-me",
		BaseDir:   tmpDir,
		CreatedAt: time.Now(),
	}
	_ = store.Save(ovl)

	if err := setLock("lock-me", true); err != nil {
		t.Fatalf("setLock(true) failed: %v", err)
	}

	loaded, _ := store.Load("lock-me")
	if !loaded.Locked {
		t.Error("overlay should be locked")
	}
}

func TestSetLock_Unlock(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	store := createTestStore(t, tmpDir)

	ovl := &api.Overlay{
		Name:      "unlock-me",
		BaseDir:   tmpDir,
		Locked:    true,
		CreatedAt: time.Now(),
	}
	_ = store.Save(ovl)

	if err := setLock("unlock-me", false); err != nil {
		t.Fatalf("setLock(false) failed: %v", err)
	}

	loaded, _ := store.Load("unlock-me")
	if loaded.Locked {
		t.Error("overlay should be unlocked")
	}
}
