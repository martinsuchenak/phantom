package commands

import (
	"context"
	"testing"
	"time"

	"github.com/martinsuchenak/phantom/pkg/api"
)

func TestPrintStatusJSON(t *testing.T) {
	ovl := &api.Overlay{
		Name:       "test",
		MountPoint: "/tmp/mnt/test",
		BaseDir:    "/tmp/base",
		UpperDir:   "/tmp/upper",
		Branch:     "feature/test",
		Persistent: false,
	}
	status := &api.OverlayStatus{
		Mounted:   true,
		Uptime:    5 * time.Minute,
		SizeBytes: 1024,
	}

	err := printStatusJSON(ovl, status)
	if err != nil {
		t.Errorf("printStatusJSON failed: %v", err)
	}
}

func TestPrintStatusTable(t *testing.T) {
	setupTestEnv(t, t.TempDir())

	ovl := &api.Overlay{
		Name:       "test",
		MountPoint: "/tmp/mnt/test",
		BaseDir:    "/tmp/base",
		UpperDir:   "/tmp/upper",
		Branch:     "feature/test",
		CreatedAt:  time.Now(),
	}
	status := &api.OverlayStatus{
		Mounted:   true,
		Uptime:    5 * time.Minute,
		SizeBytes: 2048,
	}

	err := printStatusTable(context.Background(), ovl, status)
	if err != nil {
		t.Errorf("printStatusTable failed: %v", err)
	}
}

func TestPrintStatusTable_Unmounted(t *testing.T) {
	setupTestEnv(t, t.TempDir())

	ovl := &api.Overlay{
		Name:       "test",
		MountPoint: "/tmp/mnt/test",
		BaseDir:    "/tmp/base",
		UpperDir:   "",
		CreatedAt:  time.Now(),
	}
	status := &api.OverlayStatus{
		Mounted: false,
	}

	err := printStatusTable(context.Background(), ovl, status)
	if err != nil {
		t.Errorf("printStatusTable unmounted failed: %v", err)
	}
}
