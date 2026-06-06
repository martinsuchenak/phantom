package remotefs

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// MountOpts configures a FUSE mount for a RemoteFS.
type MountOpts struct {
	MountPoint string
	AllowOther bool
	// ReadyCh is closed once the FUSE server has successfully mounted.
	ReadyCh chan<- struct{}
	// ReadyFile is written once the mount is ready (cross-process signalling).
	ReadyFile string
}

func Mount(ctx context.Context, rfs *RemoteFS, opts MountOpts) error {
	if err := os.MkdirAll(opts.MountPoint, 0755); err != nil {
		return fmt.Errorf("create mount point %s: %w", opts.MountPoint, err)
	}

	// Clear any stale FUSE mount left by a previous crashed daemon.
	// This is a no-op when nothing is mounted.
	_ = exec.Command("umount", "-f", opts.MountPoint).Run()

	root := newRootNode(rfs)
	fuseOpts := &fs.Options{
		MountOptions: fuse.MountOptions{
			AllowOther: opts.AllowOther,
			FsName:     "phantom-remote",
			Name:       "phantom",
		},
	}

	server, err := fs.Mount(opts.MountPoint, root, fuseOpts)
	if err != nil {
		return fmt.Errorf("fuse mount %s: %w", opts.MountPoint, err)
	}

	if opts.ReadyCh != nil {
		close(opts.ReadyCh)
	}
	if opts.ReadyFile != "" {
		_ = os.WriteFile(opts.ReadyFile, []byte("ready"), 0600)
	}

	<-ctx.Done()
	if opts.ReadyFile != "" {
		_ = os.Remove(opts.ReadyFile)
	}
	return server.Unmount()
}
