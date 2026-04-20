//go:build integration

package mdns

import (
	"testing"
	"time"
)

// TestAnnounceAndDiscover verifies the full announce→discover round-trip on
// the local loopback multicast interface. Requires multicast UDP to be
// available (not available in all CI environments).
//
// Run with: go test -tags integration ./internal/mdns/
func TestAnnounceAndDiscover(t *testing.T) {
	srv, err := Announce("test-node", 59876, []string{"testrepo"})
	if err != nil {
		t.Fatalf("Announce: %v", err)
	}
	defer srv.Close()

	peers, err := Discover(2 * time.Second)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	var found bool
	for _, p := range peers {
		if p.ID != "test-node" {
			continue
		}
		found = true
		if len(p.Repos) != 1 || p.Repos[0] != "testrepo" {
			t.Errorf("repos: got %v, want [testrepo]", p.Repos)
		}
	}
	if !found {
		t.Error("Discover did not return the announced node")
	}
}
