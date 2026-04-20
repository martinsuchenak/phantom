package remotefs

import (
	"context"

	proto "github.com/martinsuchenak/phantom/internal/rpc/proto"
	"github.com/martinsuchenak/phantom/internal/rpc"
)

type RemoteFS struct {
	inner proto.FileServiceClient
	repo  string
}

func NewRemoteFS(client proto.FileServiceClient, repo string) *RemoteFS {
	return &RemoteFS{inner: client, repo: repo}
}

func NewRemoteFSFromDial(ctx context.Context, addr string, opts rpc.DialOpts, repo string) (*RemoteFS, error) {
	fc, err := rpc.Dial(ctx, addr, opts)
	if err != nil {
		return nil, err
	}
	return &RemoteFS{inner: fc.Inner(), repo: repo}, nil
}

type AttrInfo struct {
	Name        string
	IsDir       bool
	Size        int64
	ModTimeUnix int64
	Mode        uint32
}

func (r *RemoteFS) GetAttr(ctx context.Context, path string) (AttrInfo, error) {
	resp, err := r.inner.Stat(ctx, &proto.StatRequest{Repo: r.repo, Path: path})
	if err != nil {
		return AttrInfo{}, err
	}
	return AttrInfo{
		Name:        resp.Name,
		IsDir:       resp.IsDir,
		Size:        resp.Size,
		ModTimeUnix: resp.ModTimeUnix,
		Mode:        resp.Mode,
	}, nil
}

func (r *RemoteFS) ListDir(ctx context.Context, path string) ([]AttrInfo, error) {
	resp, err := r.inner.ReadDir(ctx, &proto.ReadDirRequest{Repo: r.repo, Path: path})
	if err != nil {
		return nil, err
	}
	out := make([]AttrInfo, len(resp.Entries))
	for i, e := range resp.Entries {
		out[i] = AttrInfo{
			Name:        e.Name,
			IsDir:       e.IsDir,
			Size:        e.Size,
			ModTimeUnix: e.ModTimeUnix,
			Mode:        e.Mode,
		}
	}
	return out, nil
}

func (r *RemoteFS) ReadBytes(ctx context.Context, path string, offset, length int64, dest []byte) (int, error) {
	stream, err := r.inner.Read(ctx, &proto.ReadRequest{
		Repo:   r.repo,
		Path:   path,
		Offset: offset,
		Length: length,
	})
	if err != nil {
		return 0, err
	}
	var n int
	for n < len(dest) {
		chunk, err := stream.Recv()
		if err != nil {
			break
		}
		copied := copy(dest[n:], chunk.Data)
		n += copied
	}
	return n, nil
}

func (r *RemoteFS) InnerClient() proto.FileServiceClient {
	return r.inner
}
