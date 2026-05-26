package remotefs

import (
	"context"
	"fmt"
	"os"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type MountOpts struct {
	MountPoint string
	AllowOther bool
	// ReadyCh is closed once the FUSE server has successfully mounted and is
	// ready to serve requests. Optional; ignored if nil.
	ReadyCh chan<- struct{}
	// ReadyFile is a file path that is written once the mount is ready.
	// Used for cross-process readiness signalling (e.g. _fuse-daemon).
	// Optional; ignored if empty.
	ReadyFile string
}

func Mount(ctx context.Context, rfs *RemoteFS, opts MountOpts) error {
	if err := os.MkdirAll(opts.MountPoint, 0755); err != nil {
		return fmt.Errorf("create mount point %s: %w", opts.MountPoint, err)
	}

	root := newRootNode(rfs)
	fuseOpts := &fs.Options{
		MountOptions: fuse.MountOptions{
			AllowOther: opts.AllowOther,
			FsName:     "phantom-remote",
			Name:       "phantom",
		},
	}

	// fs.Mount blocks until the kernel acknowledges the mount, so the server
	// is ready to serve as soon as this returns without error.
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
