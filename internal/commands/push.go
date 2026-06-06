package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/martinsuchenak/phantom/internal/rpc"
	"github.com/martinsuchenak/phantom/internal/state"
	synckit "github.com/martinsuchenak/phantom/internal/sync"
	phantomtsnet "github.com/martinsuchenak/phantom/internal/tsnet"
	"github.com/martinsuchenak/phantom/pkg/api"
	"github.com/paularlott/cli"
)

func NewPushCommand() *cli.Command {
	return &cli.Command{
		Name:        "push",
		Usage:       "Push overlay changes to a remote node",
		Description: "Syncs the upper directory of a local overlay to a remote node via gRPC.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "message",
				Aliases: []string{"m"},
				Usage:   "Commit message for the remote sync",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "name",
				Usage:    "Name of the overlay to push",
				Required: true,
			},
		},
		Run: doPush,
	}
}

func ValidatePushOverlay(ovl *api.Overlay) error {
	if !ovl.Remote {
		return fmt.Errorf("overlay %q is not a remote overlay", ovl.Name)
	}
	if ovl.RemoteNode == "" {
		return fmt.Errorf("overlay %q has no remote node configured", ovl.Name)
	}
	if ovl.RemoteRepo == "" {
		return fmt.Errorf("overlay %q has no remote repo configured", ovl.Name)
	}
	return nil
}

func doPush(ctx context.Context, cmd *cli.Command) error {
	name := cmd.GetStringArg("name")
	message := cmd.GetString("message")

	if message == "" {
		message = fmt.Sprintf("phantom push from overlay %s", name)
	}

	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	ovl, err := store.Load(name)
	if err != nil {
		return err
	}

	if err := ValidatePushOverlay(ovl); err != nil {
		return err
	}

	// ovl.RemoteNode is stored as host:port during `phantom start --repo`.
	dialOpts := rpc.DialOpts{
		Auth:   rpc.AuthOptions{Mode: rpc.AuthMode(cfg.Node.Auth.Mode), Secret: cfg.Node.Auth.Secret},
		CAFile: cfg.Node.Auth.CAFile,
		Cert:   cfg.Node.Auth.CertFile,
		Key:    cfg.Node.Auth.KeyFile,
	}

	if cfg.Node.Tsnet.Hostname != "" {
		tsnetCfg := phantomtsnet.Config{
			Hostname:   cfg.Node.Tsnet.Hostname,
			Dir:        cfg.TsnetDirOrDefault(),
			AuthKey:    cfg.Node.Tsnet.AuthKey,
			ControlURL: cfg.Node.Tsnet.ControlURL,
		}
		srv, err := setupClientTsnet(ctx, tsnetCfg)
		if err != nil {
			return fmt.Errorf("tsnet setup: %w", err)
		}
		defer func() { _ = srv.Close() }()

		dialer := phantomtsnet.NewSmartDialer(srv, 10*time.Second)
		dialOpts.ContextDialer = dialer.DialContext
	}

	client, err := rpc.Dial(ctx, ovl.RemoteNode, dialOpts)
	if err != nil {
		return fmt.Errorf("dial remote node %q: %w", ovl.RemoteNode, err)
	}

	syncer := synckit.NewSyncer(
		client.Inner(),
		ovl.RemoteRepo,
		cfg.Node.Sync.MaxFileSizeBytes,
	)

	result, err := syncer.Push(ctx, ovl.UpperDir, message)
	if err != nil {
		return fmt.Errorf("push failed: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("push unsuccessful: %s", result.Error)
	}

	log.Info("Pushed overlay %q to %s/%s", name, ovl.RemoteNode, ovl.RemoteRepo)
	if result.GitCommitted {
		log.Info("Remote git commit: %s", result.GitCommitHash)
	}

	return nil
}
