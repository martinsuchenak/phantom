//go:build integration

package node_test

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/paularlott/gossip"
	"github.com/paularlott/gossip/codec"
	"github.com/paularlott/logger"

	"github.com/martinsuchenak/phantom/internal/node"
	"github.com/martinsuchenak/phantom/pkg/api"
)

func allocPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("alloc port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func startTestNode(t *testing.T, ctx context.Context, id string, gossipPort int, seeds []string, repos []string, registry *node.Registry) {
	t.Helper()
	errCh := make(chan error, 1)
	go func() {
		errCh <- node.Start(ctx, node.Config{
			ID:       id,
			BindAddr: fmt.Sprintf("127.0.0.1:%d", gossipPort),
			GRPCAddr: fmt.Sprintf("127.0.0.1:%d", gossipPort),
			Seeds:    seeds,
			Repos:    repos,
			PIDFile:  "",
			Logger:   NewTestLogger(t, id[:8]),
		}, registry)
	}()

	go func() {
		select {
		case err := <-errCh:
			if err != nil && ctx.Err() == nil {
				t.Errorf("node %s exited unexpectedly: %v", id[:8], err)
			}
		case <-time.After(30 * time.Second):
		}
	}()

	time.Sleep(50 * time.Millisecond)
}

type testLogger struct {
	t      *testing.T
	prefix string
}

func NewTestLogger(t *testing.T, prefix string) *testLogger {
	return &testLogger{t: t, prefix: prefix}
}

func (l *testLogger) Trace(msg string, args ...any) {}
func (l *testLogger) Debug(msg string, args ...any) {
	l.t.Logf("[DEBUG][%s] %s", l.prefix, fmt.Sprintf(msg, args...))
}
func (l *testLogger) Info(msg string, args ...any) {
	l.t.Logf("[INFO][%s] %s", l.prefix, fmt.Sprintf(msg, args...))
}
func (l *testLogger) Warn(msg string, args ...any) {
	l.t.Logf("[WARN][%s] %s", l.prefix, fmt.Sprintf(msg, args...))
}
func (l *testLogger) Error(msg string, args ...any) {
	l.t.Logf("[ERROR][%s] %s", l.prefix, fmt.Sprintf(msg, args...))
}
func (l *testLogger) Fatal(msg string, args ...any) {
	l.t.Fatalf("[FATAL][%s] %s", l.prefix, fmt.Sprintf(msg, args...))
}
func (l *testLogger) With(string, any) logger.Logger { return l }
func (l *testLogger) WithError(error) logger.Logger  { return l }
func (l *testLogger) WithGroup(string) logger.Logger { return l }

func TestGossip_TwoNodesPeerDiscovery(t *testing.T) {
	regA := node.NewRegistry()
	regB := node.NewRegistry()

	portA := allocPort(t)
	portB := allocPort(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	idA := uuid.New().String()
	idB := uuid.New().String()

	startTestNode(t, ctx, idA, portA, nil, []string{"myapp"}, regA)
	startTestNode(t, ctx, idB, portB, []string{fmt.Sprintf("127.0.0.1:%d", portA)}, []string{"otherapp"}, regB)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		peers := regB.All()
		for _, p := range peers {
			if p.ID == idA {
				goto found
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("node B never discovered node A")

found:
	peer := regB.FindByRepo("myapp")
	if len(peer) == 0 {
		t.Fatal("node B should see myapp repo from node A")
	}
	if peer[0].ID != idA {
		t.Errorf("expected peer ID %s, got %s", idA, peer[0].ID)
	}

	peersA := regA.All()
	if len(peersA) == 0 {
		t.Fatal("node A should see node B")
	}

	t.Logf("node A registry: %d peers, node B registry: %d peers", len(regA.All()), len(regB.All()))
}

func TestGossip_ThreeNodesConverge(t *testing.T) {
	regA := node.NewRegistry()
	regB := node.NewRegistry()
	regC := node.NewRegistry()

	portA := allocPort(t)
	portB := allocPort(t)
	portC := allocPort(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	idA := uuid.New().String()
	idB := uuid.New().String()
	idC := uuid.New().String()

	startTestNode(t, ctx, idA, portA, nil, []string{"repo-a"}, regA)
	startTestNode(t, ctx, idB, portB, []string{fmt.Sprintf("127.0.0.1:%d", portA)}, []string{"repo-b"}, regB)
	startTestNode(t, ctx, idC, portC, []string{fmt.Sprintf("127.0.0.1:%d", portA)}, []string{"repo-c"}, regC)

	waitForPeers(t, regC, 2, 10*time.Second)

	peers := regC.All()
	ids := map[string]bool{}
	for _, p := range peers {
		ids[p.ID] = true
	}
	if !ids[idA] {
		t.Error("node C should see node A")
	}
	if !ids[idB] {
		t.Error("node C should see node B (gossip-forwarded)")
	}

	allRepos := map[string]bool{}
	for _, p := range regC.All() {
		for _, r := range p.Repos {
			allRepos[r] = true
		}
	}
	if !allRepos["repo-a"] || !allRepos["repo-b"] {
		t.Errorf("node C should see repo-a and repo-b, got: %v", allRepos)
	}

	t.Logf("node C sees %d peers, repos: %v", len(peers), allRepos)
}

func TestGossip_NodeLeaveRemovesFromRegistry(t *testing.T) {
	regA := node.NewRegistry()
	regB := node.NewRegistry()

	portA := allocPort(t)
	portB := allocPort(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)

	idA := uuid.New().String()
	idB := uuid.New().String()

	startTestNode(t, ctx, idA, portA, nil, []string{"myapp"}, regA)
	startTestNode(t, ctx, idB, portB, []string{fmt.Sprintf("127.0.0.1:%d", portA)}, []string{"other"}, regB)

	waitForPeers(t, regB, 1, 10*time.Second)

	cancel()
	time.Sleep(500 * time.Millisecond)

	portA2 := allocPort(t)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel2()

	regA2 := node.NewRegistry()
	startTestNode(t, ctx2, idA, portA2, nil, []string{"myapp"}, regA2)

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		hasB := false
		for _, p := range regA2.All() {
			if p.ID == idB {
				hasB = true
			}
		}
		if !hasB {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Error("node B should have been removed from node A's registry after leaving")
}

func TestGossip_MetadataPropagation(t *testing.T) {
	regA := node.NewRegistry()
	regB := node.NewRegistry()

	portA := allocPort(t)
	portB := allocPort(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	idA := uuid.New().String()
	idB := uuid.New().String()

	startTestNode(t, ctx, idA, portA, nil, []string{"app1", "app2"}, regA)
	startTestNode(t, ctx, idB, portB, []string{fmt.Sprintf("127.0.0.1:%d", portA)}, []string{"app3"}, regB)

	waitForPeers(t, regB, 1, 10*time.Second)

	peerA := regB.FindByRepo("app1")
	if len(peerA) == 0 {
		t.Fatal("node B should see app1 repo from node A")
	}
	if peerA[0].ID != idA {
		t.Errorf("expected peer ID %s for app1, got %s", idA, peerA[0].ID)
	}

	peerA2 := regB.FindByRepo("app2")
	if len(peerA2) == 0 {
		t.Fatal("node B should see app2 repo from node A")
	}

	peerB := regA.FindByRepo("app3")
	if len(peerB) == 0 {
		t.Fatal("node A should see app3 repo from node B")
	}
	if peerB[0].ID != idB {
		t.Errorf("expected peer ID %s for app3, got %s", idB, peerB[0].ID)
	}
}

func TestGossip_PeersStateRoundTrip(t *testing.T) {
	reg := node.NewRegistry()
	reg.Upsert(api.Peer{ID: "node-1", GRPCAddr: "10.0.0.1:50051", Repos: []string{"app1"}})
	reg.Upsert(api.Peer{ID: "node-2", GRPCAddr: "10.0.0.2:50051", Repos: []string{"app2", "app3"}})

	tmpDir := t.TempDir()
	statePath := tmpDir + "/peers.json"

	err := node.WritePeersState(statePath, "self-id", reg)
	if err != nil {
		t.Fatalf("WritePeersState: %v", err)
	}

	state, err := node.ReadPeersState(statePath)
	if err != nil {
		t.Fatalf("ReadPeersState: %v", err)
	}

	if state.SelfID != "self-id" {
		t.Errorf("expected self_id 'self-id', got %q", state.SelfID)
	}
	if len(state.Peers) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(state.Peers))
	}

	found := map[string]bool{}
	for _, p := range state.Peers {
		found[p.ID] = true
	}
	if !found["node-1"] || !found["node-2"] {
		t.Errorf("expected node-1 and node-2, got %v", found)
	}
}

func TestGossip_RegistryFindByRepoEmpty(t *testing.T) {
	reg := node.NewRegistry()
	result := reg.FindByRepo("nonexistent")
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func waitForPeers(t *testing.T, reg *node.Registry, count int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(reg.All()) >= count {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d peers (have %d)", count, len(reg.All()))
}

func TestGossip_DirectClusterAPI(t *testing.T) {
	port1 := allocPort(t)
	port2 := allocPort(t)

	cfg1 := gossip.DefaultConfig()
	cfg1.NodeID = uuid.New().String()
	cfg1.BindAddr = fmt.Sprintf("127.0.0.1:%d", port1)
	cfg1.AdvertiseAddr = cfg1.BindAddr
	cfg1.MsgCodec = codec.NewJsonCodec()
	cfg1.Logger = logger.NewNullLogger()
	cfg1.Transport = gossip.NewSocketTransport(cfg1)

	cluster1, err := gossip.NewCluster(cfg1)
	if err != nil {
		t.Fatalf("create cluster1: %v", err)
	}
	cluster1.Start()

	cfg2 := gossip.DefaultConfig()
	cfg2.NodeID = uuid.New().String()
	cfg2.BindAddr = fmt.Sprintf("127.0.0.1:%d", port2)
	cfg2.AdvertiseAddr = cfg2.BindAddr
	cfg2.MsgCodec = codec.NewJsonCodec()
	cfg2.Logger = logger.NewNullLogger()
	cfg2.Transport = gossip.NewSocketTransport(cfg2)

	cluster2, err := gossip.NewCluster(cfg2)
	if err != nil {
		t.Fatalf("create cluster2: %v", err)
	}
	cluster2.Start()

	cluster2.LocalMetadata().SetString("test_key", "hello_from_2")

	if err := cluster2.Join([]string{fmt.Sprintf("127.0.0.1:%d", port1)}); err != nil {
		t.Fatalf("join: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cluster1.NumAliveNodes() >= 2 && cluster2.NumAliveNodes() >= 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if cluster1.NumAliveNodes() < 2 {
		t.Fatalf("cluster1 expected 2 alive nodes, got %d", cluster1.NumAliveNodes())
	}
	if cluster2.NumAliveNodes() < 2 {
		t.Fatalf("cluster2 expected 2 alive nodes, got %d", cluster2.NumAliveNodes())
	}

	nodes := cluster1.Nodes()
	var foundRemote bool
	for _, n := range nodes {
		if n.ID != cluster1.LocalNode().ID {
			foundRemote = true
			t.Logf("cluster1 sees node %s at %s", n.ID, n.AdvertisedAddr())
		}
	}
	if !foundRemote {
		t.Error("cluster1 should see at least one remote node")
	}

	cluster2.Leave()
	cluster1.Leave()
}
