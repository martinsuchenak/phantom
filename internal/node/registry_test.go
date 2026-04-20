package node_test

import (
	"testing"

	"github.com/martinsuchenak/phantom/internal/node"
	"github.com/martinsuchenak/phantom/pkg/api"
)

func TestRegistryAddAndLookup(t *testing.T) {
	r := node.NewRegistry()
	p := api.Peer{ID: "node-1", GRPCAddr: "localhost:5000", Repos: []string{"repo-a"}}
	r.Upsert(p)

	all := r.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(all))
	}
	if all[0].ID != "node-1" {
		t.Errorf("expected ID node-1, got %s", all[0].ID)
	}
	if all[0].GRPCAddr != "localhost:5000" {
		t.Errorf("expected GRPCAddr localhost:5000, got %s", all[0].GRPCAddr)
	}
}

func TestRegistryFindByRepoMultiple(t *testing.T) {
	r := node.NewRegistry()
	r.Upsert(api.Peer{ID: "a", Repos: []string{"repo1", "repo2"}})
	r.Upsert(api.Peer{ID: "b", Repos: []string{"repo2", "repo3"}})
	r.Upsert(api.Peer{ID: "c", Repos: []string{"repo4"}})

	found := r.FindByRepo("repo2")
	if len(found) != 2 {
		t.Fatalf("expected 2 peers for repo2, got %d", len(found))
	}

	ids := map[string]bool{}
	for _, p := range found {
		ids[p.ID] = true
	}
	if !ids["a"] || !ids["b"] {
		t.Errorf("expected peers a and b, got %v", ids)
	}

	foundNone := r.FindByRepo("nonexistent")
	if len(foundNone) != 0 {
		t.Errorf("expected 0 peers for nonexistent repo, got %d", len(foundNone))
	}
}

func TestRegistryRemove(t *testing.T) {
	r := node.NewRegistry()
	r.Upsert(api.Peer{ID: "x", Repos: []string{"r1"}})
	r.Upsert(api.Peer{ID: "y", Repos: []string{"r2"}})

	if len(r.All()) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(r.All()))
	}

	r.Remove("x")
	all := r.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 peer after remove, got %d", len(all))
	}
	if all[0].ID != "y" {
		t.Errorf("expected remaining peer y, got %s", all[0].ID)
	}

	r.Remove("nonexistent")
	if len(r.All()) != 1 {
		t.Error("removing nonexistent ID should not affect peers")
	}
}

func TestRegistryAll(t *testing.T) {
	r := node.NewRegistry()
	if all := r.All(); len(all) != 0 {
		t.Errorf("expected empty, got %d", len(all))
	}

	r.Upsert(api.Peer{ID: "p1", Repos: []string{"r"}})
	r.Upsert(api.Peer{ID: "p2", Repos: []string{"r"}})
	r.Upsert(api.Peer{ID: "p3", Repos: []string{"r"}})

	all := r.All()
	if len(all) != 3 {
		t.Errorf("expected 3, got %d", len(all))
	}
}

func TestRegistryUpsertUpdates(t *testing.T) {
	r := node.NewRegistry()
	r.Upsert(api.Peer{ID: "n1", GRPCAddr: "old:5000", Repos: []string{"r1"}})
	r.Upsert(api.Peer{ID: "n1", GRPCAddr: "new:6000", Repos: []string{"r1", "r2"}})

	all := r.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 peer after upsert, got %d", len(all))
	}
	if all[0].GRPCAddr != "new:6000" {
		t.Errorf("expected updated GRPCAddr new:6000, got %s", all[0].GRPCAddr)
	}
	if len(all[0].Repos) != 2 {
		t.Errorf("expected 2 repos, got %d", len(all[0].Repos))
	}
}
