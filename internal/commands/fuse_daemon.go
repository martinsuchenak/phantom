package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/martinsuchenak/phantom/internal/remotefs"
	phantomrpc "github.com/martinsuchenak/phantom/internal/rpc"
	phantomtsnet "github.com/martinsuchenak/phantom/internal/tsnet"
	"github.com/paularlott/cli"
	"tailscale.com/tsnet"
)

func NewFuseDaemonCommand() *cli.Command {
	return &cli.Command{
		Name:        "_fuse-daemon",
		Usage:       "Internal: maintain a remote FUSE mount (do not call directly)",
		Description: "Started by phantom start --repo as a detached process. Mounts the remote gRPC filesystem at the given mountpoint and blocks until terminated.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "addr",
				Usage:    "gRPC address of the remote node (host:port)",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "repo",
				Usage:    "Remote repo name to mount",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "mountpoint",
				Usage:    "Local path to mount the remote filesystem",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "ready-file",
				Usage: "Path of file to create once the mount is ready",
			},
			&cli.StringFlag{
				Name:  "auth-mode",
				Usage: "Auth mode: none | secret | mtls",
			},
			&cli.StringFlag{
				Name:  "auth-secret",
				Usage: "Shared secret (auth-mode=secret)",
			},
			&cli.StringFlag{
				Name:  "auth-cert",
				Usage: "TLS certificate file (auth-mode=mtls)",
			},
			&cli.StringFlag{
				Name:  "auth-key",
				Usage: "TLS key file (auth-mode=mtls)",
			},
			&cli.StringFlag{
				Name:  "auth-ca",
				Usage: "CA certificate file (auth-mode=mtls)",
			},
			&cli.StringFlag{
				Name:  "tsnet-hostname",
				Usage: "Tailscale hostname (enables tsnet transport)",
			},
			&cli.StringFlag{
				Name:  "tsnet-dir",
				Usage: "Tsnet state directory",
			},
			&cli.StringFlag{
				Name:  "tsnet-authkey",
				Usage: "Tailscale auth key",
			},
			&cli.StringFlag{
				Name:  "tsnet-controlurl",
				Usage: "Custom coordination server URL",
			},
		},
		Run: doFuseDaemon,
	}
}

func doFuseDaemon(ctx context.Context, cmd *cli.Command) error {
	addr := cmd.GetString("addr")
	repo := cmd.GetString("repo")
	mountpoint := cmd.GetString("mountpoint")
	readyFile := cmd.GetString("ready-file")

	authOpts := phantomrpc.DialOpts{
		Auth: phantomrpc.AuthOptions{
			Mode:   phantomrpc.AuthMode(cmd.GetString("auth-mode")),
			Secret: cmd.GetString("auth-secret"),
		},
		Cert:   cmd.GetString("auth-cert"),
		Key:    cmd.GetString("auth-key"),
		CAFile: cmd.GetString("auth-ca"),
	}

	tsnetHostname := cmd.GetString("tsnet-hostname")
	if tsnetHostname != "" {
		tsnetCfg := phantomtsnet.Config{
			Hostname:   tsnetHostname,
			Dir:        cmd.GetString("tsnet-dir"),
			AuthKey:    cmd.GetString("tsnet-authkey"),
			ControlURL: cmd.GetString("tsnet-controlurl"),
		}
		srv, err := setupClientTsnet(ctx, tsnetCfg)
		if err != nil {
			return fmt.Errorf("tsnet setup: %w", err)
		}
		defer func() { _ = srv.Close() }()

		dialer := phantomtsnet.NewSmartDialer(srv, 10*time.Second)
		authOpts.ContextDialer = dialer.DialContext
	}

	rfs, err := remotefs.NewRemoteFSFromDial(ctx, addr, authOpts, repo)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", addr, err)
	}

	return remotefs.Mount(ctx, rfs, remotefs.MountOpts{
		MountPoint: mountpoint,
		ReadyFile:  readyFile,
		AllowOther: true,
	})
}

func setupClientTsnet(ctx context.Context, cfg phantomtsnet.Config) (*tsnet.Server, error) {
	srv := &tsnet.Server{
		Hostname:   cfg.Hostname,
		Dir:        cfg.Dir,
		AuthKey:    cfg.AuthKey,
		ControlURL: cfg.ControlURL,
		Ephemeral:  cfg.AuthKey != "",
	}
	if err := srv.Start(); err != nil {
		return nil, fmt.Errorf("tsnet start: %w", err)
	}
	if _, err := srv.Up(ctx); err != nil {
		_ = srv.Close()
		return nil, fmt.Errorf("tsnet up: %w", err)
	}
	return srv, nil
}
