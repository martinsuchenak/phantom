package commands

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/martinsuchenak/phantom/internal/config"
	phantommdns "github.com/martinsuchenak/phantom/internal/mdns"
	"github.com/martinsuchenak/phantom/internal/node"
	"github.com/martinsuchenak/phantom/internal/rpc"
	proto "github.com/martinsuchenak/phantom/internal/rpc/proto"
	"github.com/martinsuchenak/phantom/internal/state"
	synckit "github.com/martinsuchenak/phantom/internal/sync"
	phantomtsnet "github.com/martinsuchenak/phantom/internal/tsnet"
	"github.com/paularlott/cli"
	"google.golang.org/grpc"
	"tailscale.com/tsnet"
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

	repos, repoNames := servedRepos(cfg)
	if len(repos) == 0 {
		log.Warn("No projects are marked as served. Use 'phantom project serve <name>' to expose a project.")
		log.Warn("The gRPC server will start but remote overlays cannot connect until a project is served.")
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

	var tsnetSrv *tsnet.Server
	var lis net.Listener

	tsnetCfg := phantomtsnet.Config{
		Hostname:   nc.Tsnet.Hostname,
		Dir:        cfg.TsnetDirOrDefault(),
		AuthKey:    nc.Tsnet.AuthKey,
		ControlURL: nc.Tsnet.ControlURL,
	}

	if phantomtsnet.IsEnabled(tsnetCfg) {
		var dl *phantomtsnet.DualListener
		srv, dl, err := phantomtsnet.Setup(ctx, tsnetCfg, grpcAddr)
		if err != nil {
			return fmt.Errorf("tsnet setup: %w", err)
		}
		tsnetSrv = srv
		lis = dl
		log.Info("tsnet enabled (hostname=%s, tailscale IPs will be logged above)", tsnetCfg.Hostname)
	} else {
		var err error
		lis, err = net.Listen("tcp", grpcAddr)
		if err != nil {
			return fmt.Errorf("listen on %s: %w", grpcAddr, err)
		}
	}

	go func() {
		log.Info("gRPC server listening on %s", grpcAddr)
		if err := grpcServer.Serve(lis); err != nil {
			log.Error("gRPC server error: %v", err)
		}
	}()

	registry := node.NewRegistry()

	// repoUpdateCh delivers updated repo slices to the gossip node on live reload.
	repoUpdateCh := make(chan []string, 1)

	gossipAddr := fmt.Sprintf(":%d", nc.GossipPort)
	nodeCfg := node.Config{
		ID:            nc.ID,
		BindAddr:      gossipAddr,
		AdvertiseAddr: fmt.Sprintf("%s:%d", outboundIP(), nc.GossipPort),
		GRPCAddr:      fmt.Sprintf("%s:%d", outboundIP(), nc.GRPCPort),
		Seeds:        nc.Seeds,
		Repos:        repoNames,
		PIDFile:      cfg.GetNodePIDPath(),
		Logger:       log,
		RepoUpdateCh: repoUpdateCh,
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

	// mDNS server with live-swap support.
	var mdnsMu sync.Mutex
	var mdnsSrv *phantommdns.Server
	if srv, err := phantommdns.Announce(nc.ID, nc.GRPCPort, repoNames); err != nil {
		log.Warn("mDNS announce failed (LAN auto-discovery disabled): %v", err)
	} else {
		mdnsSrv = srv
	}
	defer func() {
		mdnsMu.Lock()
		if mdnsSrv != nil {
			mdnsSrv.Close()
		}
		mdnsMu.Unlock()
	}()

	// Watch config file for changes and hot-reload served projects.
	go watchConfigReload(ctx, nc, fileServer, repoUpdateCh, &mdnsMu, &mdnsSrv)

	startRemoteSentinels(ctx, nc, tsnetSrv)

	log.Info("Starting gossip node %s on %s", nc.ID, gossipAddr)
	if err := node.Start(ctx, nodeCfg, registry); err != nil {
		grpcServer.GracefulStop()
		return fmt.Errorf("gossip node stopped: %w", err)
	}

	grpcServer.GracefulStop()
	if tsnetSrv != nil {
		_ = tsnetSrv.Close()
	}
	return nil
}

// servedRepos extracts the repos map and names slice from config.
func servedRepos(c *config.Config) (map[string]string, []string) {
	repos := make(map[string]string)
	names := make([]string, 0)
	for name, proj := range c.Projects {
		if proj.Serve {
			repos[name] = proj.Path
			names = append(names, name)
		}
	}
	return repos, names
}

// watchConfigReload watches the config file and hot-reloads served projects.
func watchConfigReload(
	ctx context.Context,
	nc config.NodeConfig,
	fileServer *rpc.FileServer,
	repoUpdateCh chan<- []string,
	mdnsMu *sync.Mutex,
	mdnsSrv **phantommdns.Server,
) {
	// Resolve the config file path (cfgPath may be empty → default location).
	watchPath := cfgPath
	if watchPath == "" {
		watchPath = config.DefaultConfigPath()
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Warn("config watcher: failed to create fsnotify watcher: %v", err)
		return
	}
	defer watcher.Close()

	// Watch the directory; file-level watches miss atomic saves (rename+create).
	if err := watcher.Add(filepath.Dir(watchPath)); err != nil {
		log.Warn("config watcher: cannot watch %s: %v", filepath.Dir(watchPath), err)
		return
	}

	var debounce <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if filepath.Clean(event.Name) != filepath.Clean(watchPath) {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			// Debounce: editors often fire multiple events in quick succession.
			debounce = time.After(300 * time.Millisecond)
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Warn("config watcher error: %v", err)
		case <-debounce:
			debounce = nil
			newCfg, err := config.Load(watchPath)
			if err != nil {
				log.Warn("config reload failed: %v", err)
				continue
			}
			newRepos, newNames := servedRepos(newCfg)
			fileServer.UpdateRepos(newRepos)

			// Non-blocking send; gossip node picks it up when ready.
			select {
			case repoUpdateCh <- newNames:
			default:
			}

			// Swap mDNS announcement.
			mdnsMu.Lock()
			if *mdnsSrv != nil {
				(*mdnsSrv).Close()
			}
			if srv, err := phantommdns.Announce(nc.ID, nc.GRPCPort, newNames); err != nil {
				log.Warn("mDNS re-announce failed: %v", err)
				*mdnsSrv = nil
			} else {
				*mdnsSrv = srv
			}
			mdnsMu.Unlock()

			log.Info("config reloaded: now serving %d project(s): %v", len(newRepos), newNames)
		}
	}
}

// startRemoteSentinels loads all remote overlays from the state store,
// restarts any dead FUSE daemons, and starts a sync sentinel watcher goroutine
// for each one so the daemon owns both the FUSE mount and the sync lifecycle.
func startRemoteSentinels(ctx context.Context, nc config.NodeConfig, tsnetSrv *tsnet.Server) {
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

	selfExe, exeErr := os.Executable()

	for _, ovl := range overlays {
		if !ovl.Remote || ovl.RemoteNode == "" || ovl.RemoteRepo == "" {
			continue
		}
		ovl := ovl

		// Restart the fuse-daemon if it died (PID gone or process no longer exists).
		if exeErr == nil && ovl.RemoteMountPath != "" {
			needsMount := true
			if ovl.FUSEPid > 0 {
				proc, err := os.FindProcess(ovl.FUSEPid)
				if err == nil && proc.Signal(syscall.Signal(0)) == nil {
					needsMount = false // process still alive
				}
			}
			if needsMount {
				readyFile := ovl.RemoteMountPath + ".fuse_ready"
				_ = os.Remove(readyFile)
				daemonArgs := []string{
					"_fuse-daemon",
					"--addr", ovl.RemoteNode,
					"--repo", ovl.RemoteRepo,
					"--mountpoint", ovl.RemoteMountPath,
					"--ready-file", readyFile,
					"--auth-mode", nc.Auth.Mode,
					"--auth-secret", nc.Auth.Secret,
				}
				if nc.Auth.CertFile != "" {
					daemonArgs = append(daemonArgs, "--auth-cert", nc.Auth.CertFile)
				}
				if nc.Auth.KeyFile != "" {
					daemonArgs = append(daemonArgs, "--auth-key", nc.Auth.KeyFile)
				}
				if nc.Auth.CAFile != "" {
					daemonArgs = append(daemonArgs, "--auth-ca", nc.Auth.CAFile)
				}
				if nc.Tsnet.Hostname != "" {
					daemonArgs = append(daemonArgs, "--tsnet-hostname", nc.Tsnet.Hostname)
					daemonArgs = append(daemonArgs, "--tsnet-dir", cfg.TsnetDirOrDefault())
					if nc.Tsnet.AuthKey != "" {
						daemonArgs = append(daemonArgs, "--tsnet-authkey", nc.Tsnet.AuthKey)
					}
					if nc.Tsnet.ControlURL != "" {
						daemonArgs = append(daemonArgs, "--tsnet-controlurl", nc.Tsnet.ControlURL)
					}
				}
				cmd := exec.CommandContext(context.Background(), selfExe, daemonArgs...)
				cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
				if startErr := cmd.Start(); startErr == nil {
					ovl.FUSEPid = cmd.Process.Pid
					if s2, err2 := state.NewStore(cfg.GetStatePath()); err2 == nil {
						if o2, err3 := s2.Load(ovl.Name); err3 == nil {
							o2.FUSEPid = cmd.Process.Pid
							_ = s2.Save(o2)
						}
					}
					log.Info("sentinel: restarted fuse-daemon for overlay %q (PID %d)", ovl.Name, cmd.Process.Pid)
				} else {
					log.Warn("sentinel: failed to restart fuse-daemon for overlay %q: %v", ovl.Name, startErr)
				}
			}
		}

		go func() {
			dialOpts := rpc.DialOpts{
				Auth:   rpc.AuthOptions{Mode: rpc.AuthMode(nc.Auth.Mode), Secret: nc.Auth.Secret},
				CAFile: nc.Auth.CAFile,
				Cert:   nc.Auth.CertFile,
				Key:    nc.Auth.KeyFile,
			}
			if tsnetSrv != nil {
				dialer := phantomtsnet.NewSmartDialer(tsnetSrv, 10*time.Second)
				dialOpts.ContextDialer = dialer.DialContext
			}
			client, err := rpc.Dial(ctx, ovl.RemoteNode, dialOpts)
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
