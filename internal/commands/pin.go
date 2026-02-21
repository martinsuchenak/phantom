package commands

import (
	"context"
	"fmt"

	"github.com/martinsuchenak/phantom/internal/git"
	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/martinsuchenak/phantom/pkg/api"
	"github.com/paularlott/cli"
)

// NewPinCommand creates the pin command
func NewPinCommand() *cli.Command {
	return &cli.Command{
		Name:  "pin",
		Usage: "Pin an overlay to the current base commit",
		Description: "Records the base directory's current commit hash. " +
			"Commands like sync and start will warn if the base has diverged from the pinned commit.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "commit",
				Usage: "Pin to a specific commit hash (default: current HEAD)",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{Name: "name", Usage: "Overlay name", Required: true},
		},
		Run: doPin,
	}
}

// NewUnpinCommand creates the unpin command
func NewUnpinCommand() *cli.Command {
	return &cli.Command{
		Name:        "unpin",
		Usage:       "Remove the commit pin from an overlay",
		Description: "Clears the pinned commit, allowing the overlay to follow base changes freely.",
		Arguments: []cli.Argument{
			&cli.StringArg{Name: "name", Usage: "Overlay name", Required: true},
		},
		Run: doUnpin,
	}
}

func doPin(ctx context.Context, cmd *cli.Command) error {
	name := cmd.GetStringArg("name")
	commitFlag := cmd.GetString("commit")

	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	ovl, err := store.Load(name)
	if err != nil {
		return err
	}

	gitOps := git.NewOperations()
	isGit, _ := gitOps.IsGitRepo(ctx, ovl.BaseDir)
	if !isGit {
		return fmt.Errorf("base directory %q is not a git repository — pin requires git", ovl.BaseDir)
	}

	commit := commitFlag
	if commit == "" {
		hash, err := gitOps.GetCommitHash(ctx, ovl.BaseDir)
		if err != nil {
			return fmt.Errorf("failed to get current commit: %w", err)
		}
		commit = hash
	}

	ovl.PinnedCommit = commit
	if err := store.Save(ovl); err != nil {
		return fmt.Errorf("failed to save overlay state: %w", err)
	}

	short := commit
	if len(short) > 10 {
		short = short[:10]
	}
	log.Info("Overlay %q pinned to commit %s", name, short)
	return nil
}

func doUnpin(ctx context.Context, cmd *cli.Command) error {
	name := cmd.GetStringArg("name")

	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	ovl, err := store.Load(name)
	if err != nil {
		return err
	}

	if ovl.PinnedCommit == "" {
		log.Info("Overlay %q is not pinned", name)
		return nil
	}

	ovl.PinnedCommit = ""
	if err := store.Save(ovl); err != nil {
		return fmt.Errorf("failed to save overlay state: %w", err)
	}

	log.Info("Overlay %q unpinned", name)
	return nil
}

// CheckPinDivergence checks if the base has moved past the pinned commit.
// Returns (diverged, currentCommit, error). If not pinned, returns (false, "", nil).
func CheckPinDivergence(ctx context.Context, ovl *api.Overlay) (bool, string, error) {
	if ovl.PinnedCommit == "" {
		return false, "", nil
	}

	gitOps := git.NewOperations()
	isGit, _ := gitOps.IsGitRepo(ctx, ovl.BaseDir)
	if !isGit {
		return false, "", nil
	}

	currentHash, err := gitOps.GetCommitHash(ctx, ovl.BaseDir)
	if err != nil {
		return false, "", err
	}

	if currentHash != ovl.PinnedCommit {
		return true, currentHash, nil
	}
	return false, currentHash, nil
}
