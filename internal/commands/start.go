package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/martinsuchenak/phantom/internal/git"
	phantommdns "github.com/martinsuchenak/phantom/internal/mdns"
	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/martinsuchenak/phantom/pkg/api"
	"github.com/paularlott/cli"
)

// NewStartCommand creates the start command
func NewStartCommand() *cli.Command {
	return &cli.Command{
		Name:        "start",
		Usage:       "Create and mount a new overlay filesystem",
		Description: "Creates a new overlay filesystem for the specified base directory and prints the mount point path.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "name",
				Aliases: []string{"n"},
				Usage:   "Name for the overlay",
				EnvVars: []string{"OVERLAY_NAME"},
			},
			&cli.StringFlag{
				Name:    "branch",
				Aliases: []string{"b"},
				Usage:   "Git branch name (default: phantom/<name>)",
				EnvVars: []string{"OVERLAY_BRANCH"},
			},
			&cli.BoolFlag{
				Name:    "persistent",
				Aliases: []string{"p"},
				Usage:   "Keep overlay data across reboots",
			},
			&cli.StringFlag{
				Name:  "repo",
				Usage: "Remote repo name to use as base (requires --node)",
			},
			&cli.StringFlag{
				Name:  "node",
				Usage: "Remote node address (host[:port]); if omitted, auto-discovered via mDNS",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:  "base-dir",
				Usage: "Base directory to overlay",
			},
		},
		Run: doStart,
	}
}

func doStart(ctx context.Context, cmd *cli.Command) error {
	baseDir := resolveBaseDir(cmd.GetStringArg("base-dir"))
	repo := cmd.GetString("repo")
	nodeAddr := cmd.GetString("node")

	if err := validateStartArgs(baseDir, repo, nodeAddr); err != nil {
		return err
	}

	name := cmd.GetString("name")
	branch := cmd.GetString("branch")
	persistent := cmd.GetBool("persistent")

	if repo != "" {
		return processStartRemote(ctx, repo, nodeAddr, name, branch, persistent)
	}

	return processStart(ctx, baseDir, name, branch, persistent)
}

func validateStartArgs(baseDir, repo, nodeAddr string) error {
	if baseDir != "" && repo != "" {
		return fmt.Errorf("specify either a base directory or --repo, not both")
	}
	if baseDir == "" && repo == "" {
		return fmt.Errorf("base directory or --repo is required")
	}
	return nil
}

func processStartRemote(ctx context.Context, repo, nodeAddr, name, branch string, persistent bool) error {
	if nodeAddr == "" {
		log.Info("--node not specified, probing LAN via mDNS for a node serving %q...", repo)
		discovered, err := phantommdns.DiscoverRepo(ctx, repo, 0)
		if err != nil {
			return fmt.Errorf("--node not provided and mDNS discovery failed: %w", err)
		}
		log.Info("Found node at %s via mDNS", discovered)
		nodeAddr = discovered
	}

	grpcAddr := nodeAddr
	if !strings.Contains(nodeAddr, ":") {
		grpcAddr = fmt.Sprintf("%s:%d", nodeAddr, cfg.Node.GRPCPort)
	}

	mountBase := cfg.GetRemoteMountsPath()
	safeNodeName := strings.ReplaceAll(grpcAddr, ":", "_")
	remoteMountPath := filepath.Join(mountBase, safeNodeName, repo)
	if err := os.MkdirAll(remoteMountPath, 0755); err != nil {
		return fmt.Errorf("create remote mount dir: %w", err)
	}

	if name == "" {
		name = repo
	}

	// Launch _fuse-daemon as a detached background process so the FUSE mount
	// outlives the phantom start process.
	selfExe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	readyFile := remoteMountPath + ".fuse_ready"
	_ = os.Remove(readyFile) // ensure stale ready file is gone

	daemonArgs := []string{
		"_fuse-daemon",
		"--addr", grpcAddr,
		"--repo", repo,
		"--mountpoint", remoteMountPath,
		"--ready-file", readyFile,
		"--auth-mode", cfg.Node.Auth.Mode,
		"--auth-secret", cfg.Node.Auth.Secret,
	}
	if cfg.Node.Auth.CertFile != "" {
		daemonArgs = append(daemonArgs, "--auth-cert", cfg.Node.Auth.CertFile)
	}
	if cfg.Node.Auth.KeyFile != "" {
		daemonArgs = append(daemonArgs, "--auth-key", cfg.Node.Auth.KeyFile)
	}
	if cfg.Node.Auth.CAFile != "" {
		daemonArgs = append(daemonArgs, "--auth-ca", cfg.Node.Auth.CAFile)
	}
	if cfg.Node.Tsnet.Hostname != "" {
		daemonArgs = append(daemonArgs, "--tsnet-hostname", cfg.Node.Tsnet.Hostname)
		daemonArgs = append(daemonArgs, "--tsnet-dir", cfg.TsnetDirOrDefault())
		if cfg.Node.Tsnet.AuthKey != "" {
			daemonArgs = append(daemonArgs, "--tsnet-authkey", cfg.Node.Tsnet.AuthKey)
		}
		if cfg.Node.Tsnet.ControlURL != "" {
			daemonArgs = append(daemonArgs, "--tsnet-controlurl", cfg.Node.Tsnet.ControlURL)
		}
	}

	fuseDaemon := exec.CommandContext(context.Background(), selfExe, daemonArgs...)
	fuseDaemon.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach from terminal
	if err := fuseDaemon.Start(); err != nil {
		return fmt.Errorf("start fuse-daemon: %w", err)
	}
	fusePID := fuseDaemon.Process.Pid

	// Poll for the ready file (up to 15s).
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(readyFile); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			_ = fuseDaemon.Process.Kill()
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if _, err := os.Stat(readyFile); err != nil {
		_ = fuseDaemon.Process.Kill()
		return fmt.Errorf("timed out waiting for FUSE mount at %s", remoteMountPath)
	}

	overlayErr := processStart(ctx, remoteMountPath, name, branch, persistent)

	if overlayErr == nil {
		store, err := state.NewStore(cfg.GetStatePath())
		if err == nil {
			if ovl, loadErr := store.Load(name); loadErr == nil {
				ovl.Remote = true
				ovl.RemoteNode = grpcAddr
				ovl.RemoteRepo = repo
				ovl.RemoteMountPath = remoteMountPath
				ovl.FUSEPid = fusePID
				_ = store.Save(ovl)
			}
		}
	}

	if overlayErr != nil {
		_ = fuseDaemon.Process.Kill()
	}

	return overlayErr
}

func processStart(ctx context.Context, baseDir, name, branch string, persistent bool) error {
	// Get absolute path for base directory
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("failed to resolve base directory path: %w", err)
	}

	// Generate name if not provided
	if name == "" {
		// Use the base directory name
		name = filepath.Base(absBaseDir)
		if name == "." || name == "/" {
			return fmt.Errorf("could not generate overlay name, please specify with -n")
		}
	}

	// Validate name format to prevent path traversal and ensure safety
	// Only allow alphanumeric characters, hyphens, and underscores
	validName := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validName.MatchString(name) {
		return fmt.Errorf("invalid overlay name %q: must contain only alphanumeric characters, hyphens, and underscores", name)
	}

	// Validate branch name if provided (prevent git injection)
	if branch != "" {
		if err := validateBranchName(branch); err != nil {
			return err
		}
	}

	log.Debug("Creating overlay %q for %q", name, absBaseDir)

	// Initialize state store
	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	// Check if overlay already exists
	if store.Exists(name) {
		return api.NewError(api.ErrAlreadyExists, fmt.Sprintf("overlay %q already exists", name), nil)
	}

	// Initialize git operations
	gitOps := git.NewOperations()

	// Check if base directory is a git repo
	isGit, err := gitOps.IsGitRepo(ctx, absBaseDir)
	if err != nil {
		log.Debug("Git check failed: %v", err)
	}

	// Generate branch name if not provided
	if branch == "" && isGit && cfg.Git.AutoBranch {
		branch = cfg.Git.BranchPrefix + name
	}

	// Create the overlay manager
	mgr, err := createOverlayManager()
	if err != nil {
		return err
	}

	// Create overlay options
	opts := &api.CreateOptions{
		Name:       name,
		BaseDir:    absBaseDir,
		Branch:     branch,
		Persistent: persistent,
	}

	// Create the overlay
	ovl, err := mgr.Create(opts)
	if err != nil {
		return err
	}

	// Track if we need to rollback
	var gitErrors []string
	stashApplied := false

	// Handle git branch creation if applicable
	if isGit && branch != "" {
		log.Debug("Creating git branch %q", branch)

		// Check for uncommitted changes
		hasChanges, err := gitOps.HasUncommittedChanges(ctx, absBaseDir)
		if err != nil {
			gitErrors = append(gitErrors, fmt.Sprintf("failed to check uncommitted changes: %v", err))
		}

		if hasChanges {
			// Warn user about auto-stashing
			log.Warn("Uncommitted changes detected, auto-stashing (stash name: overlay-auto-stash-%s)", name)

			// Stash changes before creating branch
			if err := gitOps.Stash(ctx, absBaseDir, "overlay-auto-stash-"+name); err != nil {
				gitErrors = append(gitErrors, fmt.Sprintf("failed to stash changes: %v", err))
			} else {
				stashApplied = true
			}
		}

		// Check if branch already exists
		branchExists, _ := gitOps.BranchExists(ctx, absBaseDir, branch)
		if branchExists {
			// Switch to existing branch
			if err := gitOps.SwitchBranch(ctx, ovl.MountPoint, branch); err != nil {
				gitErrors = append(gitErrors, fmt.Sprintf("failed to switch to branch %s: %v", branch, err))
			}
		} else {
			// Create new branch in the overlay mount
			if err := gitOps.CreateBranch(ctx, ovl.MountPoint, branch, ""); err != nil {
				gitErrors = append(gitErrors, fmt.Sprintf("failed to create branch %s: %v", branch, err))
			}
		}

		if stashApplied {
			// Pop stash in the overlay
			if err := gitOps.StashPop(ctx, ovl.MountPoint); err != nil {
				gitErrors = append(gitErrors, fmt.Sprintf("failed to pop stash: %v", err))
				log.Warn("Stashed changes remain in base repo, run 'git stash pop' manually if needed")
			}
		}
	}

	// Report git errors at warn level so users see them
	for _, gitErr := range gitErrors {
		log.Warn("Git: %s", gitErr)
	}

	// Save state
	if err := store.Save(ovl); err != nil {
		// Try to cleanup on failure
		mgr.Cleanup(ovl)
		return fmt.Errorf("failed to save overlay state: %w", err)
	}

	// Output the mount point path (this is what scripts can capture)
	log.Info(ovl.MountPoint)

	return nil
}

// validateBranchName checks if a branch name is safe to use with git
func validateBranchName(branch string) error {
	// Reject branch names that could be interpreted as git options
	if strings.HasPrefix(branch, "-") {
		return fmt.Errorf("invalid branch name %q: cannot start with '-'", branch)
	}

	// Reject branch names with potentially dangerous characters
	invalidChars := []string{"..", "~", "^", ":", "?", "*", "[", "\\", " ", "\t", "\n"}
	for _, char := range invalidChars {
		if strings.Contains(branch, char) {
			return fmt.Errorf("invalid branch name %q: contains invalid character %q", branch, char)
		}
	}

	// Reject empty or whitespace-only names
	if strings.TrimSpace(branch) == "" {
		return fmt.Errorf("branch name cannot be empty")
	}

	// Reject names ending with .lock
	if strings.HasSuffix(branch, ".lock") {
		return fmt.Errorf("invalid branch name %q: cannot end with '.lock'", branch)
	}

	return nil
}
