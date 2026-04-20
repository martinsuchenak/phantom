// Package mdns provides LAN-local phantom node discovery via mDNS/DNS-SD.
//
// Nodes advertise themselves under the "_phantom._tcp" service type. The TXT
// record encodes two fields:
//
//	id=<nodeID>          — the node's logical identifier from config
//	repos=repo1,repo2    — comma-separated list of repos served by this node
//
// Any node on the same L2 segment can call Discover (or DiscoverRepo) to find
// peers without requiring explicit seed addresses in the config.
package mdns

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/hashicorp/mdns"
	"github.com/martinsuchenak/phantom/pkg/api"
)

const service = "_phantom._tcp"

// DefaultTimeout is the mDNS query timeout used when none is specified.
const DefaultTimeout = 3 * time.Second

// Server is a running mDNS announcer. Call Close when the node shuts down to
// withdraw the advertisement from the LAN.
type Server struct {
	srv *mdns.Server
}

// Close stops the mDNS announcer and releases its sockets.
func (s *Server) Close() error {
	return s.srv.Shutdown()
}

// Announce registers this node on the LAN via mDNS/DNS-SD. It advertises
// nodeID, grpcPort, and the repos list under the "_phantom._tcp" service type.
// Returns a Server that must be closed when the node shuts down.
//
// A non-fatal error (e.g. multicast not available) should be logged as a
// warning rather than aborting startup — the node will still work with
// explicit seeds.
func Announce(nodeID string, grpcPort int, repos []string) (*Server, error) {
	txt := []string{
		"id=" + nodeID,
		"repos=" + strings.Join(repos, ","),
	}
	svc, err := mdns.NewMDNSService(nodeID, service, "", "", grpcPort, nil, txt)
	if err != nil {
		return nil, fmt.Errorf("create mDNS service: %w", err)
	}
	srv, err := mdns.NewServer(&mdns.Config{Zone: svc})
	if err != nil {
		return nil, fmt.Errorf("start mDNS server: %w", err)
	}
	return &Server{srv: srv}, nil
}

// Discover queries the LAN for phantom nodes and returns all peers that
// respond within timeout. Peers whose TXT record carries no node ID are
// silently ignored. Pass 0 for timeout to use DefaultTimeout.
func Discover(timeout time.Duration) ([]api.Peer, error) {
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	entries := make(chan *mdns.ServiceEntry, 32)
	errCh := make(chan error, 1)
	go func() {
		errCh <- mdns.Query(&mdns.QueryParam{
			Service:     service,
			Timeout:     timeout,
			Entries:     entries,
			DisableIPv6: true, // IPv6 mDNS is often blocked; IPv4 is sufficient
		})
		close(entries)
	}()

	var peers []api.Peer
	for entry := range entries {
		if p, ok := parsePeer(entry); ok {
			peers = append(peers, p)
		}
	}
	if err := <-errCh; err != nil {
		return peers, fmt.Errorf("mDNS query: %w", err)
	}
	return peers, nil
}

// DiscoverRepo queries the LAN for a phantom node serving the named repo and
// returns its gRPC address (host:port). Returns an error if no node responds
// within timeout. Pass 0 for timeout to use DefaultTimeout.
func DiscoverRepo(_ context.Context, repo string, timeout time.Duration) (string, error) {
	peers, err := Discover(timeout)
	if err != nil {
		return "", err
	}
	for _, p := range peers {
		for _, r := range p.Repos {
			if r == repo {
				return p.GRPCAddr, nil
			}
		}
	}
	return "", fmt.Errorf("no phantom node found on LAN serving repo %q (use --node to specify one explicitly)", repo)
}

// parsePeer converts an mDNS service entry into an api.Peer.
// Returns false when the entry carries no usable node ID.
func parsePeer(entry *mdns.ServiceEntry) (api.Peer, bool) {
	var id string
	var repos []string
	for _, kv := range entry.InfoFields {
		switch {
		case strings.HasPrefix(kv, "id="):
			id = strings.TrimPrefix(kv, "id=")
		case strings.HasPrefix(kv, "repos="):
			r := strings.TrimPrefix(kv, "repos=")
			if r != "" {
				repos = strings.Split(r, ",")
			}
		}
	}
	if id == "" {
		return api.Peer{}, false
	}
	ip := entry.AddrV4
	if ip == nil {
		ip = entry.AddrV6
	}
	if ip == nil {
		return api.Peer{}, false
	}
	grpcAddr := net.JoinHostPort(ip.String(), fmt.Sprintf("%d", entry.Port))
	return api.Peer{ID: id, GRPCAddr: grpcAddr, Repos: repos}, true
}
