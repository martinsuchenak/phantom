package sync_test

import (
	"os"
	"path/filepath"
	"testing"

	phantomsync "github.com/martinsuchenak/phantom/internal/sync"
)

func TestWalkerRegularFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}

	changes, err := phantomsync.WalkUpperDir(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Path != "hello.txt" {
		t.Errorf("expected path hello.txt, got %s", changes[0].Path)
	}
	if string(changes[0].Data) != "world" {
		t.Errorf("expected data 'world', got %q", string(changes[0].Data))
	}
	if changes[0].Deleted || changes[0].IsDir {
		t.Error("expected regular file change")
	}
}

func TestWalkerWhiteoutFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".wh.deleted.txt"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	changes, err := phantomsync.WalkUpperDir(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Path != "deleted.txt" {
		t.Errorf("expected path deleted.txt, got %s", changes[0].Path)
	}
	if !changes[0].Deleted {
		t.Error("expected Deleted=true for whiteout file")
	}
}

func TestWalkerNestedDir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "nested.txt"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	changes, err := phantomsync.WalkUpperDir(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change (file only, not dir), got %d", len(changes))
	}
	if changes[0].Path != "sub/nested.txt" {
		t.Errorf("expected path sub/nested.txt, got %s", changes[0].Path)
	}
}

func TestWalkerEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "emptydir"), 0755); err != nil {
		t.Fatal(err)
	}

	changes, err := phantomsync.WalkUpperDir(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Errorf("expected 0 changes for empty dirs (non-opaque), got %d", len(changes))
	}
}

func TestWalkerFileSizeLimit(t *testing.T) {
	dir := t.TempDir()
	bigData := make([]byte, 2000)
	smallData := []byte("hi")
	if err := os.WriteFile(filepath.Join(dir, "big.txt"), bigData, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "small.txt"), smallData, 0644); err != nil {
		t.Fatal(err)
	}

	changes, err := phantomsync.WalkUpperDir(dir, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change (small file only), got %d", len(changes))
	}
	if changes[0].Path != "small.txt" {
		t.Errorf("expected small.txt, got %s", changes[0].Path)
	}
}
