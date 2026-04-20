package commands

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/martinsuchenak/phantom/internal/config"
	phantommdns "github.com/martinsuchenak/phantom/internal/mdns"
	"github.com/martinsuchenak/phantom/internal/node"
	"github.com/martinsuchenak/phantom/internal/rpc"
	proto "github.com/martinsuchenak/phantom/internal/rpc/proto"
	"github.com/martinsuchenak/phantom/internal/state"
	synckit "github.com/martinsuchenak/phantom/internal/sync"
	"github.com/paularlott/cli"
	"google.golang.org/grpc"
)

func NewNodeCommand() *cli.Command {
	return &cli.Command{
		Name:        "node",
		Usage:       "Manage the phantom cluster node",
		Description: "Commands for starting, stopping, and inspecting the gossip + gRPC node daemon.",
		Commands: []*cli.Command{
			NewNodeStartCommand(),
			NewNodeStopCommand(),
			NewNodeListCommand(),
		},
	}
}

func NewNodeStartCommand() *cli.Command {
	return &cli.Command{
		Name:        "start",
		Usage:       "Start the gRPC server and gossip node daemon",
		Description: "Starts the phantom node daemon which runs a gRPC file server and a gossip-based cluster for peer discovery.",
		Run:         doNodeStart,
	}
}

func doNodeStart(ctx context.Context, cmd *cli.Command) error {
	if changes := cfg.EnsureNodeDefaults(); len(changes) > 0 {
		log.Info("Node config was incomplete — applied defaults:")
		for _, c := range changes {
			log.Info("  %s", c)
		}
		if err := cfg.Save(cfgPath); err != nil {
			log.Warn("Could not save updated config: %v", err)
		} else {
			log.Info("Config written to %s", cfgPath)
		}
	}

	nc := cfg.Node

	repos := make(map[string]string)
	repoNames := make([]string, 0)
	for name, proj := range cfg.Projects {
		if proj.Serve {
			repos[name] = proj.Path
			repoNames = append(repoNames, name)
		}
	}

	if len(repos) == 0 {
		return fmt.Errorf(
			"no projects are marked as served — mark at least one project:\n\n" +
				"  phantom project serve <name>\n\n" +
				"or in config:\n\n" +
				"  projects:\n" +
				"    myapp:\n" +
				"      path: /path/to/myapp\n" +
				"      serve: true\n",
		)
	}

	fileServer := rpc.NewFileServerWithOptions(repos, nc.Sync.AutoGitCommit, nc.Sync.MaxFileSizeBytes)

	authOpts := rpc.AuthOptions{Mode: rpc.AuthMode(nc.Auth.Mode), Secret: nc.Auth.Secret}
	serverOpts := []grpc.ServerOption{
		grpc.UnaryInterceptor(rpc.UnaryAuthInterceptor(authOpts)),
		grpc.StreamInterceptor(rpc.StreamAuthInterceptor(authOpts)),
	}
	grpcServer := grpc.NewServer(serverOpts...)
	proto.RegisterFileServiceServer(grpcServer, fileServer)

	grpcAddr := fmt.Sprintf(":%d", nc.GRPCPort)
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", grpcAddr, err)
	}

	go func() {
		log.Info("gRPC server listening on %s", grpcAddr)
		if err := grpcServer.Serve(lis); err != nil {
			log.Error("gRPC server error: %v", err)
		}
	}()

	registry := node.NewRegistry()

	gossipAddr := fmt.Sprintf(":%d", nc.GossipPort)
	nodeCfg := node.Config{
		ID:       nc.ID,
		BindAddr: gossipAddr,
		GRPCAddr: fmt.Sprintf("%s:%d", outboundIP(), nc.GRPCPort),
		Seeds:    nc.Seeds,
		Repos:    repoNames,
		PIDFile:  cfg.GetNodePIDPath(),
	}

	peersStatePath := cfg.GetPeersStatePath()
	if err := node.WritePeersState(peersStatePath, nc.ID, registry); err != nil {
		log.Debug("failed to write initial peers state: %v", err)
	}
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := node.WritePeersState(peersStatePath, nc.ID, registry); err != nil {
					log.Debug("failed to write peers state: %v", err)
				}
			}
		}
	}()

	mdnsSrv, err := phantommdns.Announce(nc.ID, nc.GRPCPort, repoNames)
	if err != nil {
		log.Warn("mDNS announce failed (LAN auto-discovery disabled): %v", err)
	} else {
		defer mdnsSrv.Close()
	}

	startRemoteSentinels(ctx, nc)

	log.Info("Starting gossip node %s on %s", nc.ID, gossipAddr)
	if err := node.Start(ctx, nodeCfg, registry); err != nil {
		grpcServer.GracefulStop()
		return fmt.Errorf("gossip node stopped: %w", err)
	}

	grpcServer.GracefulStop()
	return nil
}

// startRemoteSentinels loads all remote overlays from the state store and
// starts a sentinel watcher goroutine for each one so that the daemon (not the
// short-lived `phantom start` process) owns the sync lifecycle.
func startRemoteSentinels(ctx context.Context, nc config.NodeConfig) {
	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		log.Debug("sentinel: failed to open state store: %v", err)
		return
	}
	overlays, err := store.LoadAll()
	if err != nil {
		log.Debug("sentinel: failed to list overlays: %v", err)
		return
	}
	for _, ovl := range overlays {
		if !ovl.Remote || ovl.RemoteNode == "" || ovl.RemoteRepo == "" {
			continue
		}
		ovl := ovl
		go func() {
			client, err := rpc.Dial(ctx, ovl.RemoteNode, rpc.DialOpts{
				Auth:   rpc.AuthOptions{Mode: rpc.AuthMode(nc.Auth.Mode), Secret: nc.Auth.Secret},
				CAFile: nc.Auth.CAFile,
				Cert:   nc.Auth.CertFile,
				Key:    nc.Auth.KeyFile,
			})
			if err != nil {
				log.Warn("sentinel: failed to dial %s for overlay %q: %v", ovl.RemoteNode, ovl.Name, err)
				return
			}
			log.Info("sentinel: watching overlay %q -> %s/%s", ovl.Name, ovl.RemoteNode, ovl.RemoteRepo)
			synckit.Watch(ctx, ovl.MountPoint, func(commitMsg string) {
				syncer := synckit.NewSyncer(client.Inner(), ovl.RemoteRepo, nc.Sync.MaxFileSizeBytes)
				result, err := syncer.Push(ctx, ovl.UpperDir, commitMsg)
				var outcome string
				if err != nil {
					outcome = "error: " + err.Error()
				} else if !result.Success {
					outcome = "error: " + result.Error
				} else {
					outcome = "ok"
				}
				synckit.WriteResult(ovl.MountPoint, outcome)
			})
		}()
	}
}

// outboundIP returns the preferred outbound IP of this machine by probing a
// UDP connection (no packets are actually sent).
func outboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "localhost"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}
