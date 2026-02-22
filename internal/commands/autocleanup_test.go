package commands

import "testing"

func TestRunAutoCleanup_DisabledWhenZero(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	// Set auto cleanup to 0 (disabled)
	cfg.Overlay.AutoCleanupDays = 0

	// Should return immediately without error
	runAutoCleanup()
}

func TestRunAutoCleanup_NoOverlays(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	cfg.Overlay.AutoCleanupDays = 7

	// Should not panic even with no overlays
	// (will fail to create overlay manager, but that's handled gracefully)
	runAutoCleanup()
}
