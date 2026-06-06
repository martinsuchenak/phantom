//go:build integration

package sync_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/martinsuchenak/phantom/internal/remotefs"
	"github.com/martinsuchenak/phantom/internal/rpc"
	proto "github.com/martinsuchenak/phantom/internal/rpc/proto"
	phantomsync "github.com/martinsuchenak/phantom/internal/sync"
)

func TestIntegration_ServerClientAndSync(t *testing.T) {
	repoDir := t.TempDir()
	os.WriteFile(filepath.Join(repoDir, "hello.txt"), []byte("hello world"), 0644)
	os.MkdirAll(filepath.Join(repoDir, "sub"), 0755)
	os.WriteFile(filepath.Join(repoDir, "sub", "nested.txt"), []byte("nested"), 0644)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	proto.RegisterFileServiceServer(srv, rpc.NewFileServer(map[string]string{"testrepo": repoDir}))
	go srv.Serve(lis)
	defer srv.Stop()

	addr := lis.Addr().String()

	t.Log("Phase 1: gRPC client reads")
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := proto.NewFileServiceClient(conn)
	fc := rpc.NewFileClient(client)

	info, err := fc.Stat(context.Background(), "testrepo", "hello.txt")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.IsDir || info.Size != 11 {
		t.Errorf("unexpected stat: %+v", info)
	}

	data, err := fc.ReadAll(context.Background(), "testrepo", "hello.txt", 0, 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got %q", data)
	}

	t.Log("Phase 2: FUSE mount")
	mountPoint := filepath.Join(t.TempDir(), "mnt")
	os.MkdirAll(mountPoint, 0755)

	rfs := remotefs.NewRemoteFS(client, "testrepo")
	fuseCtx, fuseCancel := context.WithCancel(context.Background())
	defer fuseCancel()

	mountReadyCh := make(chan struct{})
	mountErrCh := make(chan error, 1)
	go func() {
		mountErrCh <- remotefs.Mount(fuseCtx, rfs, remotefs.MountOpts{MountPoint: mountPoint, ReadyCh: mountReadyCh})
	}()

	select {
	case <-mountReadyCh:
	case err := <-mountErrCh:
		t.Fatalf("mount failed: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for FUSE mount")
	}

	fused, err := os.ReadFile(filepath.Join(mountPoint, "hello.txt"))
	if err != nil {
		t.Fatalf("fuse read: %v", err)
	}
	if string(fused) != "hello world" {
		t.Errorf("fuse: expected 'hello world', got %q", fused)
	}

	fuseCancel()
	select {
	case <-mountErrCh:
	case <-time.After(5 * time.Second):
		t.Error("FUSE unmount timed out")
	}

	t.Log("Phase 3: Sync push")
	serverDir2 := t.TempDir()
	lis2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen2: %v", err)
	}
	srv2 := grpc.NewServer()
	proto.RegisterFileServiceServer(srv2, rpc.NewFileServer(map[string]string{"r": serverDir2}))
	go srv2.Serve(lis2)
	defer srv2.Stop()

	conn2, err := grpc.NewClient(lis2.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial2: %v", err)
	}
	defer conn2.Close()
	client2 := proto.NewFileServiceClient(conn2)

	upper := t.TempDir()
	os.WriteFile(filepath.Join(upper, "synced.txt"), []byte("synced content"), 0644)
	os.WriteFile(filepath.Join(upper, ".wh.removed.txt"), []byte{}, 0000)

	syncer := phantomsync.NewSyncer(client2, "r", 0)
	result, err := syncer.Push(context.Background(), upper, "integration test")
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if !result.Success {
		t.Fatalf("push failed: %s", result.Error)
	}

	synced, err := os.ReadFile(filepath.Join(serverDir2, "synced.txt"))
	if err != nil {
		t.Fatalf("read synced file: %v", err)
	}
	if string(synced) != "synced content" {
		t.Errorf("expected 'synced content', got %q", synced)
	}
}
