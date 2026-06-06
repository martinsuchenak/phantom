package sync_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/martinsuchenak/phantom/internal/rpc"
	proto "github.com/martinsuchenak/phantom/internal/rpc/proto"
	phantomsync "github.com/martinsuchenak/phantom/internal/sync"
)

func setupSyncServer(t *testing.T, repos map[string]string) (proto.FileServiceClient, func()) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	proto.RegisterFileServiceServer(srv, rpc.NewFileServer(repos))
	go func() { _ = srv.Serve(lis) }()

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return proto.NewFileServiceClient(conn), func() {
		_ = conn.Close()
		srv.Stop()
	}
}

func TestSyncerPushFile(t *testing.T) {
	serverDir := t.TempDir()
	client, cleanup := setupSyncServer(t, map[string]string{"testrepo": serverDir})
	defer cleanup()

	upperDir := t.TempDir()
	_ = os.Mkdir(filepath.Join(upperDir, "sub"), 0755)
	_ = os.WriteFile(filepath.Join(upperDir, "sub", "file.txt"), []byte("hello sync"), 0644)

	syncer := phantomsync.NewSyncer(client, "testrepo", 0)
	result, err := syncer.Push(context.Background(), upperDir, "test commit")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("expected success")
	}

	data, err := os.ReadFile(filepath.Join(serverDir, "sub", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello sync" {
		t.Errorf("expected 'hello sync', got %q", string(data))
	}
}

func TestSyncerDeleteFile(t *testing.T) {
	serverDir := t.TempDir()
	targetFile := filepath.Join(serverDir, "todelete.txt")
	_ = os.WriteFile(targetFile, []byte("gonna be deleted"), 0644)

	client, cleanup := setupSyncServer(t, map[string]string{"testrepo": serverDir})
	defer cleanup()

	upperDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(upperDir, ".wh.todelete.txt"), []byte(""), 0644)

	syncer := phantomsync.NewSyncer(client, "testrepo", 0)
	result, err := syncer.Push(context.Background(), upperDir, "delete commit")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("expected success")
	}

	if _, err := os.Stat(targetFile); !os.IsNotExist(err) {
		t.Error("expected file to be deleted")
	}
}
