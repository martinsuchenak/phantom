//go:build !windows

package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

const whiteoutPrefix = ".wh."

type Change struct {
	Path    string
	Data    []byte
	Deleted bool
	IsDir   bool
	Opaque  bool
}

func WalkUpperDir(upperDir string, maxFileSizeBytes int64) ([]Change, error) {
	var changes []Change
	err := filepath.Walk(upperDir, func(abs string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if abs == upperDir {
			return nil
		}
		rel, err := filepath.Rel(upperDir, abs)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		name := info.Name()

		// Native Linux overlayfs whiteout: character device with major/minor 0:0.
		if info.Mode()&(os.ModeCharDevice|os.ModeDevice) != 0 {
			if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Rdev == 0 {
				changes = append(changes, Change{Path: rel, Deleted: true})
			}
			return nil
		}

		// AuFS-style whiteout files (.wh.<name>).
		if strings.HasPrefix(name, whiteoutPrefix) {
			originalName := name[len(whiteoutPrefix):]
			dir := filepath.Dir(rel)
			originalRel := filepath.ToSlash(filepath.Join(dir, originalName))
			if originalRel == "." {
				originalRel = originalName
			}
			changes = append(changes, Change{Path: originalRel, Deleted: true})
			return nil
		}

		if info.IsDir() {
			opaque := isOpaqueDir(abs)
			if opaque {
				changes = append(changes, Change{Path: rel, IsDir: true, Opaque: true})
			}
			return nil
		}

		if maxFileSizeBytes > 0 && info.Size() > maxFileSizeBytes {
			return nil
		}

		data, err := os.ReadFile(abs)
		if err != nil {
			return fmt.Errorf("read %s: %w", rel, err)
		}
		changes = append(changes, Change{Path: rel, Data: data})
		return nil
	})
	return changes, err
}

func isOpaqueDir(path string) bool {
	var val [1]byte
	_, err := unix.Getxattr(path, "trusted.overlay.opaque", val[:0])
	if err != nil {
		return false
	}
	return true
}
