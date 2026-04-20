package remotefs_test

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc"

	proto "github.com/martinsuchenak/phantom/internal/rpc/proto"
	"github.com/martinsuchenak/phantom/internal/remotefs"
)

type mockClient struct {
	statFn    func(ctx context.Context, req *proto.StatRequest, opts ...grpc.CallOption) (*proto.StatResponse, error)
	readDirFn func(ctx context.Context, req *proto.ReadDirRequest, opts ...grpc.CallOption) (*proto.ReadDirResponse, error)
}

func (m *mockClient) ListRepos(_ context.Context, _ *proto.ListReposRequest, _ ...grpc.CallOption) (*proto.ListReposResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockClient) Stat(ctx context.Context, req *proto.StatRequest, opts ...grpc.CallOption) (*proto.StatResponse, error) {
	return m.statFn(ctx, req, opts...)
}

func (m *mockClient) ReadDir(ctx context.Context, req *proto.ReadDirRequest, opts ...grpc.CallOption) (*proto.ReadDirResponse, error) {
	return m.readDirFn(ctx, req, opts...)
}

func (m *mockClient) Read(_ context.Context, _ *proto.ReadRequest, _ ...grpc.CallOption) (proto.FileService_ReadClient, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockClient) SyncFiles(_ context.Context, _ ...grpc.CallOption) (proto.FileService_SyncFilesClient, error) {
	return nil, fmt.Errorf("not implemented")
}

func TestRemoteFSGetattr(t *testing.T) {
	mc := &mockClient{
		statFn: func(_ context.Context, req *proto.StatRequest, _ ...grpc.CallOption) (*proto.StatResponse, error) {
			if req.Repo != "myrepo" {
				t.Errorf("expected repo myrepo, got %q", req.Repo)
			}
			if req.Path != "some/file.txt" {
				t.Errorf("expected path some/file.txt, got %q", req.Path)
			}
			return &proto.StatResponse{
				Name:        "file.txt",
				IsDir:       false,
				Size:        42,
				ModTimeUnix: 1700000000,
				Mode:        0644,
			}, nil
		},
	}

	rfs := remotefs.NewRemoteFS(mc, "myrepo")
	info, err := rfs.GetAttr(context.Background(), "some/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "file.txt" {
		t.Errorf("expected Name file.txt, got %q", info.Name)
	}
	if info.IsDir {
		t.Error("expected IsDir=false")
	}
	if info.Size != 42 {
		t.Errorf("expected Size 42, got %d", info.Size)
	}
	if info.ModTimeUnix != 1700000000 {
		t.Errorf("expected ModTimeUnix 1700000000, got %d", info.ModTimeUnix)
	}
	if info.Mode != 0644 {
		t.Errorf("expected Mode 0644, got %o", info.Mode)
	}
}

func TestRemoteFSReadDir(t *testing.T) {
	mc := &mockClient{
		readDirFn: func(_ context.Context, req *proto.ReadDirRequest, _ ...grpc.CallOption) (*proto.ReadDirResponse, error) {
			if req.Repo != "myrepo" {
				t.Errorf("expected repo myrepo, got %q", req.Repo)
			}
			if req.Path != "somedir" {
				t.Errorf("expected path somedir, got %q", req.Path)
			}
			return &proto.ReadDirResponse{
				Entries: []*proto.StatResponse{
					{Name: "a.txt", IsDir: false, Size: 10, Mode: 0644},
					{Name: "subdir", IsDir: true, Size: 0, Mode: 0755},
				},
			}, nil
		},
	}

	rfs := remotefs.NewRemoteFS(mc, "myrepo")
	entries, err := rfs.ListDir(context.Background(), "somedir")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Name != "a.txt" {
		t.Errorf("expected Name a.txt, got %q", entries[0].Name)
	}
	if entries[0].IsDir {
		t.Error("expected a.txt IsDir=false")
	}
	if !entries[1].IsDir {
		t.Error("expected subdir IsDir=true")
	}
	if entries[1].Name != "subdir" {
		t.Errorf("expected Name subdir, got %q", entries[1].Name)
	}
}
