package commands

import (
	"context"
	"fmt"

	phantomrpc "github.com/martinsuchenak/phantom/internal/rpc"
	"github.com/martinsuchenak/phantom/internal/remotefs"
	"github.com/paularlott/cli"
)

// NewFuseDaemonCommand returns the hidden _fuse-daemon command.
// It is exec'd as a detached background process by processStartRemote to keep
// a remote FUSE mount alive independently of the phantom start lifetime.
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

	rfs, err := remotefs.NewRemoteFSFromDial(ctx, addr, authOpts, repo)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", addr, err)
	}

	return remotefs.Mount(ctx, rfs, remotefs.MountOpts{
		MountPoint: mountpoint,
		ReadyFile:  readyFile,
	})
}
