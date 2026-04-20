package rpc_test

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	proto "github.com/martinsuchenak/phantom/internal/rpc/proto"
	"github.com/martinsuchenak/phantom/internal/rpc"
)

const bufSize = 1024 * 1024

func setupServer(t *testing.T, repos map[string]string) (proto.FileServiceClient, func()) {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	proto.RegisterFileServiceServer(srv, rpc.NewFileServer(repos))
	go srv.Serve(lis)

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to dial bufconn: %v", err)
	}
	return proto.NewFileServiceClient(conn), func() {
		conn.Close()
		srv.Stop()
	}
}

func TestListRepos(t *testing.T) {
	repos := map[string]string{"myapp": t.TempDir()}
	client, cleanup := setupServer(t, repos)
	defer cleanup()

	resp, err := client.ListRepos(context.Background(), &proto.ListReposRequest{})
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	if len(resp.Repos) != 1 || resp.Repos[0].Name != "myapp" {
		t.Errorf("expected [myapp], got %v", resp.Repos)
	}
}

func TestStat_File(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hi"), 0644)
	client, cleanup := setupServer(t, map[string]string{"r": dir})
	defer cleanup()

	resp, err := client.Stat(context.Background(), &proto.StatRequest{Repo: "r", Path: "hello.txt"})
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if resp.IsDir {
		t.Error("expected file, got dir")
	}
	if resp.Size != 2 {
		t.Errorf("expected size 2, got %d", resp.Size)
	}
}

func TestStat_Dir(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "sub"), 0755)
	client, cleanup := setupServer(t, map[string]string{"r": dir})
	defer cleanup()

	resp, err := client.Stat(context.Background(), &proto.StatRequest{Repo: "r", Path: "sub"})
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !resp.IsDir {
		t.Error("expected dir, got file")
	}
}

func TestStat_UnknownRepo(t *testing.T) {
	client, cleanup := setupServer(t, map[string]string{})
	defer cleanup()

	_, err := client.Stat(context.Background(), &proto.StatRequest{Repo: "nope", Path: "x"})
	if err == nil {
		t.Error("expected error for unknown repo")
	}
}

func TestStat_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	secretDir := t.TempDir()
	os.WriteFile(filepath.Join(secretDir, "secret.txt"), []byte("secret"), 0644)

	client, cleanup := setupServer(t, map[string]string{"r": dir})
	defer cleanup()

	_, err := client.Stat(context.Background(), &proto.StatRequest{Repo: "r", Path: "../../secret.txt"})
	if err == nil {
		t.Error("expected error for path traversal")
	}
}

func TestReadDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.Mkdir(filepath.Join(dir, "sub"), 0755)
	client, cleanup := setupServer(t, map[string]string{"r": dir})
	defer cleanup()

	resp, err := client.ReadDir(context.Background(), &proto.ReadDirRequest{Repo: "r", Path: ""})
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(resp.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(resp.Entries))
	}
}

func TestRead(t *testing.T) {
	dir := t.TempDir()
	content := []byte("hello world")
	os.WriteFile(filepath.Join(dir, "f.txt"), content, 0644)
	client, cleanup := setupServer(t, map[string]string{"r": dir})
	defer cleanup()

	stream, err := client.Read(context.Background(), &proto.ReadRequest{Repo: "r", Path: "f.txt", Offset: 0, Length: 0})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var got []byte
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("stream.Recv: %v", err)
		}
		got = append(got, chunk.Data...)
	}
	if string(got) != string(content) {
		t.Errorf("expected %q, got %q", content, got)
	}
}

func TestRead_WithOffset(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hello world"), 0644)
	client, cleanup := setupServer(t, map[string]string{"r": dir})
	defer cleanup()

	stream, err := client.Read(context.Background(), &proto.ReadRequest{Repo: "r", Path: "f.txt", Offset: 6, Length: 5})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var got []byte
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, chunk.Data...)
	}
	if string(got) != "world" {
		t.Errorf("expected 'world', got %q", got)
	}
}
