package remotefs

import (
	"context"
	"io"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type RemoteNode struct {
	fs.Inode
	rfs  *RemoteFS
	path string
}

var (
	_ fs.NodeGetattrer = (*RemoteNode)(nil)
	_ fs.NodeLookuper  = (*RemoteNode)(nil)
	_ fs.NodeReaddirer = (*RemoteNode)(nil)
	_ fs.NodeOpener    = (*RemoteNode)(nil)
)

func (n *RemoteNode) Getattr(ctx context.Context, _ fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	info, err := n.rfs.GetAttr(ctx, n.path)
	if err != nil {
		return syscall.EIO
	}
	out.Size = uint64(info.Size)
	out.Mtime = uint64(info.ModTimeUnix)
	out.Atime = uint64(info.ModTimeUnix)
	out.Ctime = uint64(info.ModTimeUnix)
	out.Mode = info.Mode
	return 0
}

func (n *RemoteNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	// go-fuse's pollHack opens this file on the root to synchronise mount
	// readiness. Return EPERM so pollHack treats it as a sandbox restriction
	// and skips gracefully, without making a gRPC call.
	if name == ".go-fuse-epoll-hack" {
		return nil, syscall.EPERM
	}
	childPath := joinPath(n.path, name)
	info, err := n.rfs.GetAttr(ctx, childPath)
	if err != nil {
		return nil, syscall.ENOENT
	}
	out.Size = uint64(info.Size)
	out.Mtime = uint64(info.ModTimeUnix)
	out.Mode = info.Mode

	var mode uint32
	if info.IsDir {
		mode = syscall.S_IFDIR | 0755
	} else {
		mode = syscall.S_IFREG | 0644
	}
	child := &RemoteNode{rfs: n.rfs, path: childPath}
	return n.NewInode(ctx, child, fs.StableAttr{Mode: mode}), 0
}

func (n *RemoteNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	entries, err := n.rfs.ListDir(ctx, n.path)
	if err != nil {
		return nil, syscall.EIO
	}
	dirEntries := make([]fuse.DirEntry, len(entries))
	for i, e := range entries {
		mode := uint32(syscall.S_IFREG)
		if e.IsDir {
			mode = syscall.S_IFDIR
		}
		dirEntries[i] = fuse.DirEntry{Name: e.Name, Mode: mode}
	}
	return fs.NewListDirStream(dirEntries), 0
}

func (n *RemoteNode) Open(_ context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	if flags&(syscall.O_WRONLY|syscall.O_RDWR|syscall.O_CREAT|syscall.O_TRUNC) != 0 {
		return nil, 0, syscall.EROFS
	}
	return &remoteFileHandle{rfs: n.rfs, path: n.path}, fuse.FOPEN_DIRECT_IO, 0
}

type remoteFileHandle struct {
	rfs  *RemoteFS
	path string
}

var _ fs.FileReader = (*remoteFileHandle)(nil)

func (fh *remoteFileHandle) Read(ctx context.Context, dest []byte, offset int64) (fuse.ReadResult, syscall.Errno) {
	n, err := fh.rfs.ReadBytes(ctx, fh.path, offset, int64(len(dest)), dest)
	if err != nil && err != io.EOF {
		return nil, syscall.EIO
	}
	return fuse.ReadResultData(dest[:n]), 0
}

func newRootNode(rfs *RemoteFS) *RemoteNode {
	return &RemoteNode{rfs: rfs, path: ""}
}

func joinPath(base, name string) string {
	if base == "" {
		return name
	}
	return base + "/" + name
}
