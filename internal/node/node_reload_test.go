//go:build integration

package node_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/martinsuchenak/phantom/internal/node"
)

func TestRepoUpdateCh(t *testing.T) {
	repoUpdateCh := make(chan []string, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	id := uuid.New().String()
	port := allocPort(t)

	go func() {
		if err := node.Start(ctx, node.Config{
			ID:           id,
			BindAddr:     fmt.Sprintf("127.0.0.1:%d", port),
			GRPCAddr:     fmt.Sprintf("127.0.0.1:%d", port),
			Repos:        []string{"initial-repo"},
			Logger:       NewTestLogger(t, id[:8]),
			RepoUpdateCh: repoUpdateCh,
		}, node.NewRegistry()); err != nil && ctx.Err() == nil {
			t.Errorf("primary node exited: %v", err)
		}
	}()

	time.Sleep(150 * time.Millisecond)

	// Push updated repo list.
	repoUpdateCh <- []string{"new-repo-a", "new-repo-b"}
	time.Sleep(100 * time.Millisecond)

	// Start an observer node seeded from the primary to read its metadata.
	port2 := allocPort(t)
	reg2 := node.NewRegistry()
	startTestNode(t, ctx, uuid.New().String(), port2,
		[]string{fmt.Sprintf("127.0.0.1:%d", port)},
		[]string{"observer"},
		reg2,
	)

	deadline := time.Now().Add(10 * time.Second)
	var gotRepos []string
	for time.Now().Before(deadline) {
		for _, p := range reg2.All() {
			if p.ID == id {
				gotRepos = p.Repos
				goto check
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("observer never saw the primary node in its registry")

check:
	repoSet := make(map[string]bool)
	for _, r := range gotRepos {
		repoSet[r] = true
	}
	if !repoSet["new-repo-a"] || !repoSet["new-repo-b"] {
		t.Errorf("expected [new-repo-a new-repo-b], got %v", gotRepos)
	}
}
