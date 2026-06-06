package sync

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

const sentinelFile = ".phantom_commit"
const resultFile = ".phantom_commit_result"

func Watch(ctx context.Context, mountPoint string, onTrigger func(commitMessage string)) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer func() { _ = watcher.Close() }()

	_ = watcher.Add(mountPoint)

	sentinel := filepath.Join(mountPoint, sentinelFile)

	if data, err := os.ReadFile(sentinel); err == nil {
		msg := strings.TrimSpace(string(data))
		_ = os.Remove(sentinel)
		onTrigger(msg)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Name != sentinel {
				continue
			}
			if event.Op&(fsnotify.Create|fsnotify.Write) == 0 {
				continue
			}
			data, err := os.ReadFile(sentinel)
			if err != nil {
				continue
			}
			msg := strings.TrimSpace(string(data))
			_ = os.Remove(sentinel)
			onTrigger(msg)
		case _, ok := <-watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

func WriteResult(mountPoint, result string) {
	_ = os.WriteFile(filepath.Join(mountPoint, resultFile), []byte(result+"\n"), 0644)
}
