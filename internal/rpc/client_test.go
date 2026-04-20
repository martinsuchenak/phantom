package rpc_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/martinsuchenak/phantom/internal/rpc"
)

func TestFileClientStat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x.txt"), []byte("abc"), 0644)

	srv, cleanup := setupServer(t, map[string]string{"r": dir})
	defer cleanup()

	fc := rpc.NewFileClient(srv)

	info, err := fc.Stat(context.Background(), "r", "x.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size != 3 {
		t.Errorf("expected size 3, got %d", info.Size)
	}
	if info.IsDir {
		t.Error("expected file")
	}
}

func TestFileClientReadDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)

	srv, cleanup := setupServer(t, map[string]string{"r": dir})
	defer cleanup()
	fc := rpc.NewFileClient(srv)

	entries, err := fc.ReadDir(context.Background(), "r", "")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestFileClientReadAll(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hello"), 0644)

	srv, cleanup := setupServer(t, map[string]string{"r": dir})
	defer cleanup()
	fc := rpc.NewFileClient(srv)

	data, err := fc.ReadAll(context.Background(), "r", "f.txt", 0, 0)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("expected 'hello', got %q", data)
	}
}
