package sync_test

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/martinsuchenak/phantom/internal/sync"
)

func TestSentinelDetectsFile(t *testing.T) {
	mountPoint := t.TempDir()
	var triggered atomic.Int32

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sync.Watch(ctx, mountPoint, func(msg string) {
		triggered.Add(1)
		if msg != "my commit msg" {
			t.Errorf("expected 'my commit msg', got %q", msg)
		}
	})

	time.Sleep(50 * time.Millisecond)
	_ = os.WriteFile(filepath.Join(mountPoint, ".phantom_commit"), []byte("my commit msg\n"), 0644)

	waitForTrigger(t, &triggered, 1, 2*time.Second)
}

func TestSentinelDeletesFileAfterFiring(t *testing.T) {
	mountPoint := t.TempDir()
	var triggered atomic.Int32

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sync.Watch(ctx, mountPoint, func(msg string) {
		triggered.Add(1)
	})

	sentinel := filepath.Join(mountPoint, ".phantom_commit")
	time.Sleep(50 * time.Millisecond)
	_ = os.WriteFile(sentinel, []byte("test\n"), 0644)

	waitForTrigger(t, &triggered, 1, 2*time.Second)

	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Error("expected sentinel file to be deleted after firing")
	}
}

func TestSentinelRespectsContextCancellation(t *testing.T) {
	mountPoint := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		sync.Watch(ctx, mountPoint, func(msg string) {})
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("Watch did not exit after context cancellation")
	}
}

func waitForTrigger(t *testing.T, counter *atomic.Int32, target int32, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if counter.Load() >= target {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for trigger; got %d, want %d", counter.Load(), target)
}
