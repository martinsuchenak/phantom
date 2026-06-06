//go:build darwin

package commands

import (
	"fmt"
	"os"
)

// macFUSE mount helper paths that go-fuse (hanwen/go-fuse) knows about.
// FUSE-T is NOT compatible: it uses NFS internally and never installs these
// binaries, so go-fuse's mount call will fail immediately.
var macFUSEPaths = []string{
	"/Library/Filesystems/macfuse.fs/Contents/Resources/mount_macfuse",
	"/Library/Filesystems/osxfuse.fs/Contents/Resources/mount_osxfuse",
}

func checkFUSEAvailable() error {
	for _, p := range macFUSEPaths {
		if _, err := os.Stat(p); err == nil {
			return nil
		}
	}
	return fmt.Errorf(
		"macFUSE is not installed — remote overlays require macFUSE. " +
			"Install from: https://osxfuse.github.io/ " +
			"Note: FUSE-T is not compatible with phantom's FUSE layer (go-fuse " +
			"talks directly to the macFUSE kernel device, which FUSE-T does not provide)",
	)
}
