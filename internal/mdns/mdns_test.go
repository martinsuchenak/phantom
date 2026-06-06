package mdns

import (
	"net"
	"testing"

	"github.com/hashicorp/mdns"
)

// makeEntry constructs a minimal ServiceEntry for parsePeer tests.
func makeEntry(addrV4 net.IP, port int, infoFields []string) *mdns.ServiceEntry {
	return &mdns.ServiceEntry{
		AddrV4:     addrV4,
		Port:       port,
		InfoFields: infoFields,
	}
}

func TestParsePeer_ValidEntry(t *testing.T) {
	entry := makeEntry(
		net.ParseIP("192.168.1.10"),
		50051,
		[]string{"id=node-a", "repos=myapp,shared"},
	)
	peer, ok := parsePeer(entry)
	if !ok {
		t.Fatal("expected parsePeer to succeed")
	}
	if peer.ID != "node-a" {
		t.Errorf("ID: got %q, want %q", peer.ID, "node-a")
	}
	if peer.GRPCAddr != "192.168.1.10:50051" {
		t.Errorf("GRPCAddr: got %q, want %q", peer.GRPCAddr, "192.168.1.10:50051")
	}
	if len(peer.Repos) != 2 || peer.Repos[0] != "myapp" || peer.Repos[1] != "shared" {
		t.Errorf("Repos: got %v, want [myapp shared]", peer.Repos)
	}
}

func TestParsePeer_MissingID(t *testing.T) {
	entry := makeEntry(
		net.ParseIP("10.0.0.1"),
		50051,
		[]string{"repos=myapp"},
	)
	_, ok := parsePeer(entry)
	if ok {
		t.Error("expected parsePeer to fail when id is missing")
	}
}

func TestParsePeer_EmptyRepos(t *testing.T) {
	entry := makeEntry(
		net.ParseIP("10.0.0.2"),
		50051,
		[]string{"id=node-b", "repos="},
	)
	peer, ok := parsePeer(entry)
	if !ok {
		t.Fatal("expected parsePeer to succeed")
	}
	if len(peer.Repos) != 0 {
		t.Errorf("expected empty repos, got %v", peer.Repos)
	}
}

func TestParsePeer_NoIP(t *testing.T) {
	entry := makeEntry(nil, 50051, []string{"id=node-c", "repos=foo"})
	_, ok := parsePeer(entry)
	if ok {
		t.Error("expected parsePeer to fail when no IP is present")
	}
}

func TestParsePeer_IPv4AddressFormat(t *testing.T) {
	cases := []struct {
		ip   string
		port int
		want string
	}{
		{"10.0.0.1", 50051, "10.0.0.1:50051"},
		{"192.168.100.200", 9999, "192.168.100.200:9999"},
	}
	for _, tc := range cases {
		entry := makeEntry(net.ParseIP(tc.ip), tc.port, []string{"id=n", "repos="})
		peer, ok := parsePeer(entry)
		if !ok {
			t.Errorf("ip=%s: expected ok", tc.ip)
			continue
		}
		if peer.GRPCAddr != tc.want {
			t.Errorf("ip=%s: got %q, want %q", tc.ip, peer.GRPCAddr, tc.want)
		}
	}
}

func TestParsePeer_SingleRepo(t *testing.T) {
	entry := makeEntry(
		net.ParseIP("10.1.2.3"),
		50051,
		[]string{"id=solo", "repos=onlyme"},
	)
	peer, ok := parsePeer(entry)
	if !ok {
		t.Fatal("expected parsePeer to succeed")
	}
	if len(peer.Repos) != 1 || peer.Repos[0] != "onlyme" {
		t.Errorf("Repos: got %v, want [onlyme]", peer.Repos)
	}
}

func TestParsePeer_ExtraUnknownFields(t *testing.T) {
	entry := makeEntry(
		net.ParseIP("10.0.0.5"),
		50051,
		[]string{"id=node-x", "repos=r1", "version=2", "unknown=xyz"},
	)
	peer, ok := parsePeer(entry)
	if !ok {
		t.Fatal("expected parsePeer to succeed despite unknown TXT fields")
	}
	if peer.ID != "node-x" {
		t.Errorf("ID: got %q", peer.ID)
	}
}
