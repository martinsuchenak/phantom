# Phantom

A Go-based CLI tool for managing overlay filesystems to enable multiple AI agents to work on the same codebase in parallel without conflicts. Each agent gets its own phantom overlay with isolated writes and its own git branch.

## Features

- **Cross-platform**: Linux (native overlayfs) + macOS (unionfs-fuse)
- **Git integration**: automatic branch per overlay
- **Optional persistence** across reboots
- **Agent wrapper** for running commands in overlay context
- **Single binary** distribution
- **Auto-cleanup**: expired overlays are automatically removed based on configurable age threshold
- **Secure by default**: restrictive file permissions, input validation

## Installation

### Homebrew (macOS/Linux)

```bash
brew tap martinsuchenak/tap
brew install phantom
```

### Download Binary

Download the latest release from [GitHub Releases](https://github.com/martinsuchenak/phantom/releases).

```bash
# Example for Linux amd64
curl -LO https://github.com/martinsuchenak/phantom/releases/latest/download/phantom_Linux_amd64.tar.gz
tar xzf phantom_Linux_amd64.tar.gz
sudo mv phantom /usr/local/bin/
```

### Build from Source

```bash
make build
```

The binary is output to `dist/phantom`.

### Build for All Platforms

```bash
make build-all
```

Outputs to `dist/`:
- `phantom-linux-amd64`
- `phantom-linux-arm64`
- `phantom-darwin-amd64`
- `phantom-darwin-arm64`

### Install from Source

```bash
make install
```

This installs the binary to `/usr/local/bin/phantom`.

### Prerequisites

**Linux:**
- Native overlayfs: requires root or appropriate capabilities
- Rootless: install fuse-overlayfs (`apt install fuse-overlayfs` or `dnf install fuse-overlayfs`)
- Auto-detection: if not root and fuse-overlayfs is available, it's used automatically

**macOS:**
- [macFUSE](https://osxfuse.github.io/) or [FUSE-T](https://github.com/macos-fuse-t/fuse-t)
- [unionfs-fuse](https://github.com/rpodgorny/unionfs-fuse)

## Usage

### Create a Phantom Overlay

```bash
# Create an overlay for a directory
phantom start /path/to/repo -n feature-a

# Output: /Users/user/.phantom/mnt/feature-a
```

The overlay mount point is printed to stdout, making it easy to capture in scripts:

```bash
PHANTOM_PATH=$(phantom start ~/myproject -n my-feature)
cd "$PHANTOM_PATH"
# Work in the isolated overlay...
```

### List Overlays

```bash
# List mounted overlays
phantom list

# List all overlays (including unmounted)
phantom list --all

# Output as JSON
phantom list --format json
```

### Show Status

```bash
# Status of specific overlay (includes file change stats)
phantom status feature-a

# Status of all overlays
phantom status

# Output as JSON
phantom status feature-a --format json
```

The single-overlay status includes file change counts (added/modified/deleted) from the overlay's upper directory.

### Stop an Overlay

```bash
# Unmount only
phantom stop feature-a

# Unmount and cleanup all data
phantom stop feature-a --cleanup

# Push changes before stopping
phantom stop feature-a --push

# Force unmount if stuck
phantom stop feature-a --force
```

### Run an Agent in Overlay Context

```bash
phantom run /path/to/repo --agent "claude code" --task "implement auth"
```

Options:
- `--agent, -a` - Agent command to run (required)
- `--task, -t` - Task description (optional, passed to agent as env var)
- `--name, -n` - Overlay name (auto-generated if not specified)
- `--branch, -b` - Git branch name
- `--timeout` - Timeout in minutes (default: from config, max: 1440)
- `--cleanup` - Cleanup overlay after completion
- `--push` - Push branch to remote on completion
- `--persist, -p` - Keep overlay mounted after completion

Environment variables set for the agent:
- `OVERLAY_NAME` - Overlay name
- `OVERLAY_PATH` - Mount point
- `OVERLAY_BASE` - Original directory
- `OVERLAY_BRANCH` - Git branch
- `OVERLAY_TASK` - Task description
- `OVERLAY_ENABLED` - Always "true"

### Run Multiple Agents in Parallel

```bash
# Using a config file
phantom run-all /path/to/repo --config agents.yaml

# Using inline agent list
phantom run-all /path/to/repo --agents "claude,copilot,aider"

# With global options
phantom run-all /path/to/repo --config agents.yaml --timeout 30 --cleanup --push

# JSON summary output
phantom run-all /path/to/repo --config agents.yaml --format json
```

Options:
- `--config, -c` - Path to agents YAML config file
- `--agents` - Comma-separated agent commands (simple mode)
- `--timeout` - Global timeout per agent in minutes (max 1440)
- `--cleanup` - Cleanup all overlays after completion
- `--push` - Push branches to remote on completion
- `--format` - Summary output format: `table` (default), `json`

Example `agents.yaml`:

```yaml
agents:
  - name: auth-agent
    agent: "claude --print"
    task: "implement authentication module"
    branch: "feature/auth"
    timeout: 30
  - name: api-agent
    agent: 'aider "{task}"'
    task: "build REST API endpoints"
    branch: "feature/api"
  - name: test-agent
    agent: 'claude "{task}"'
    task: "write unit tests"
```

Each agent gets its own overlay and git branch. All agents run concurrently in headless mode (no terminal stdin). A summary table is printed at the end showing exit codes, durations, and pass/fail status. Agent output is logged to `~/.phantom/logs/<name>.log`.

Task delivery to agents (in parallel mode):
- **`{task}` placeholder**: If the `agent` string contains `{task}`, it's replaced with the task text. Example: `agent: 'claude "{task}"'`
- **stdin piping**: If no `{task}` placeholder is used but a `task` is defined, the task text is piped to the agent's stdin. This works with agents like `claude --print` that read from stdin.
- **Inline prompt**: You can also embed the prompt directly in the agent command: `agent: 'claude "implement auth"'`

### Show Changes in an Overlay

```bash
# Show all files changed in an overlay
phantom diff feature-a

# Summary statistics only
phantom diff feature-a --stat

# Output as JSON
phantom diff feature-a --format json

# Simple output (git-style A/M/D prefixes)
phantom diff feature-a --format simple
```

Options:
- `--format` - Output format: `table` (default), `json`, `simple`
- `--stat` - Show summary statistics only (added/modified/deleted counts)

### Prune Stale Overlays

```bash
# Remove all unmounted and expired overlays
phantom prune

# Preview what would be removed
phantom prune --dry-run

# Also unmount and remove expired overlays that are still mounted
phantom prune --force
```

Options:
- `--dry-run` - Show what would be removed without actually removing
- `--force, -f` - Also remove mounted but expired overlays (will unmount them)

Persistent overlays are always skipped. The expiry threshold is controlled by `overlay.auto_cleanup_days` in the config (default: 7 days).

### Commit Changes in an Overlay

```bash
# Commit all changes
phantom commit feature-a -m "implement auth module"

# Commit and push to remote
phantom commit feature-a -m "implement auth module" --push
```

Options:
- `--message, -m` - Commit message (required)
- `--push` - Push to remote after committing

The overlay must be mounted and be a git repository. All changes are staged automatically (`git add -A`).

### Apply Overlay Changes to Base Directory

```bash
# Merge overlay branch into base repo (git)
phantom apply feature-a

# Preview what would be applied
phantom apply feature-a --dry-run

# Apply and stop the overlay
phantom apply feature-a --stop

# Apply, stop, and cleanup
phantom apply feature-a --cleanup
```

Options:
- `--dry-run` - Show what would be applied without making changes
- `--stop` - Stop the overlay after applying
- `--cleanup` - Cleanup overlay data after applying (implies --stop)

For git repos, this auto-commits any uncommitted changes in the overlay, then merges the overlay branch into the base repo's current branch. For non-git directories, it compares the overlay mount point against the base to copy new/modified files and remove deleted ones. The overlay must be mounted.

### View Agent Logs

```bash
# Show full log for an overlay
phantom logs feature-a

# Show last 4096 bytes
phantom logs feature-a --tail 4096

# Print the log file path (useful for piping)
phantom logs feature-a --path
```

Options:
- `--tail, -n` - Show last N bytes (0 = entire file)
- `--path` - Print the log file path instead of contents

Agent logs are stored in `~/.phantom/logs/<name>.log` and include stdout/stderr from `phantom run` executions. Each run appends to the log with a timestamped header.

### Restart an Overlay

```bash
# Remount an overlay that was unmounted (e.g. after reboot)
phantom restart feature-a
```

If the overlay is already mounted, this is a no-op. The overlay state must still exist in `~/.phantom/state/`.

### Initialize Configuration

```bash
# Create default config.yaml and example agents.yaml
phantom init

# Overwrite existing files
phantom init --force
```

Creates `~/.phantom/config.yaml` with documented defaults and `~/.phantom/agents.yaml` with example agent definitions. Existing files are skipped unless `--force` is used.

### Health Check

```bash
# Check health of all overlays and system
phantom health

# Attempt to fix detected issues (remount stale overlays)
phantom health --fix

# JSON output
phantom health --format json
```

Options:
- `--fix` - Attempt to fix issues (remount stale overlays, clean zombies)
- `--format` - Output format: `table` (default), `json`

Checks: FUSE availability, overlay mount status, base/upper directory existence, FUSE process liveness, zombie mount detection.

### Overlay Snapshots

```bash
# Save a snapshot of an overlay
phantom snapshot save feature-a
phantom snapshot save feature-a -s "before-refactor"

# List snapshots
phantom snapshot list
phantom snapshot list feature-a
phantom snapshot list --format json

# Restore a snapshot (overlay must be stopped first)
phantom stop feature-a
phantom snapshot restore feature-a feature-a-20260220-143000
phantom restart feature-a

# Delete a snapshot
phantom snapshot delete feature-a-20260220-143000
```

Snapshots copy the overlay's upper directory (all changes) to `~/.phantom/snapshots/<name>/`. Useful for saving a known-good state before risky experiments. The overlay must be unmounted before restoring.

### Shell Completion

```bash
# Bash
source <(phantom completion bash)

# Zsh
source <(phantom completion zsh)

# Fish
phantom completion fish | source

# PowerShell
phantom completion powershell | Out-String | Invoke-Expression
```

Supports dynamic completion for commands, subcommands, and flags.

### Using .env Files

Phantom automatically loads `.env` files from the current directory. This allows you to set default values for flags:

```bash
# .env file
OVERLAY_AGENT=claude
OVERLAY_NAME=my-feature
OVERLAY_TASK=implement authentication
OVERLAY_BRANCH=feature/auth
```

Then run without specifying flags:

```bash
phantom run ./src/
```

Supported environment variables:
- `OVERLAY_CONFIG` - Config file path
- `OVERLAY_NAME` - Overlay name
- `OVERLAY_AGENT` - Agent command
- `OVERLAY_TASK` - Task description
- `OVERLAY_BASE` - Base directory
- `OVERLAY_BRANCH` - Git branch name

The `.env` file supports:
- Comments (`# comment`)
- Variable expansion (`${VAR}` or `$VAR`)
- Quoted values (single and double quotes)

## Commands

| Command | Description |
|---------|-------------|
| `phantom start <base-dir>` | Create overlay, print mount path |
| `phantom stop <name>` | Unmount and optionally cleanup/push |
| `phantom list` | List all active overlays |
| `phantom status [<name>]` | Show overlay state |
| `phantom diff <name>` | Show files changed in an overlay |
| `phantom commit <name> -m "msg"` | Commit changes in an overlay |
| `phantom apply <name>` | Apply overlay changes to base directory |
| `phantom logs <name>` | Show agent execution logs |
| `phantom restart <name>` | Remount an unmounted overlay |
| `phantom prune` | Remove stale and expired overlays |
| `phantom run <base-dir> --agent <cmd>` | Run agent in overlay context |
| `phantom run-all <base-dir>` | Run multiple agents in parallel |
| `phantom init` | Initialize default configuration |
| `phantom health` | Check health of overlays and system |
| `phantom snapshot save <name>` | Save a snapshot of an overlay |
| `phantom snapshot restore <name> <snap>` | Restore overlay from snapshot |
| `phantom snapshot list [<name>]` | List snapshots |
| `phantom snapshot delete <snap>` | Delete a snapshot |
| `phantom completion <shell>` | Generate shell completion scripts |

### Global Flags

- `--config, -c` - Config file path (default: `~/.phantom/config.yaml`)
- `--verbose, -v` - Verbose output

## Configuration

Configuration is stored in `~/.phantom/config.yaml`:

```yaml
state_dir: "~/.phantom"
logging:
  level: info                    # trace, debug, info, warn, error, fatal
  file: "~/.phantom/phantom.log"
overlay:
  persistent: false
  auto_cleanup_days: 7
git:
  auto_branch: true
  branch_prefix: "phantom/"
  auto_push_on_stop: false
darwin:
  unionfs_path: ""               # auto-detect
  fuse_options:
    - "cow"
linux:
  use_fuse: false                # auto-detects if not root; set true to force
  fuse_overlay_path: ""          # auto-detect fuse-overlayfs
  fuse_options: []
agent:
  default_timeout_minutes: 60    # max: 1440 (24 hours)
  cleanup_on_success: true
  cleanup_on_failure: false
agent_env:
  - "OVERLAY_ENABLED=true"
```

### Configuration Validation

The configuration is validated on load:
- `logging.level` must be one of: trace, debug, info, warn, error, fatal
- `agent.default_timeout_minutes` must be between 0 and 1440
- `overlay.auto_cleanup_days` cannot be negative (set to 0 to disable auto-cleanup)
- `state_dir` cannot be empty

### Auto-Cleanup Behavior

The `overlay.auto_cleanup_days` setting (default: 7) controls automatic garbage collection of old overlays. When `phantom start` or `phantom run` is executed, Phantom silently removes overlays that are:
- Older than the configured threshold
- Not currently mounted
- Not marked as persistent

Set `auto_cleanup_days` to `0` to disable. For manual control, use `phantom prune`.

## How It Works

### Linux (Native)

Uses native kernel overlayfs via syscall (requires root):

```
mount -t overlay overlay \
  -o lowerdir=/base,upperdir=/upper,workdir=/work \
  /mnt/point
```

### Linux (FUSE - Rootless)

Uses fuse-overlayfs for unprivileged operation (auto-detected when not root):

```
fuse-overlayfs -o lowerdir=/base,upperdir=/upper,workdir=/work /mnt/point
```

Force with `linux.use_fuse: true` in config.

### macOS

Uses unionfs-fuse (requires macFUSE or FUSE-T):

```
unionfs-fuse -o cow upperdir=RW:lowerdir=RO /mnt/point
```

## Directory Structure

```
~/.phantom/
├── config.yaml          # Configuration (0600 permissions)
├── phantom.log          # Log file
├── state/               # Overlay state files (0600 permissions)
│   ├── feature-a.json
│   └── feature-b.json
├── overlays/            # Overlay data (0700 permissions)
│   ├── feature-a/
│   │   ├── upper/       # Writable layer
│   │   └── work/        # Work directory (Linux only)
│   └── feature-b/
├── mnt/                 # Mount points (0700 permissions)
│   ├── feature-a/
│   └── feature-b/
└── logs/                # Agent execution logs (0700 permissions)
    ├── feature-a.log
    └── feature-b.log
└── snapshots/           # Overlay snapshots (0700 permissions)
    └── feature-a-20260220-143000/
        ├── meta.json
        └── data/        # Copy of upper directory at snapshot time
```

## Security

Phantom implements several security measures:

- **File permissions**: State and config files use 0600, directories use 0700
- **Input validation**: Overlay names restricted to alphanumeric, hyphens, underscores
- **Branch name validation**: Git branch names validated to prevent injection
- **Symlink protection**: Base directories cannot be symlinks
- **Path injection prevention**: Paths with commas rejected (overlayfs option injection)
- **Process verification**: PIDs verified before killing (macOS)
- **File locking**: State files use file locking to prevent race conditions
- **No shell injection**: Agent commands parsed into arguments, not passed to shell

## Error Codes

| Code | Description |
|------|-------------|
| `MOUNT_FAILED` | Failed to mount overlay |
| `UNMOUNT_FAILED` | Failed to unmount |
| `NOT_FOUND` | Overlay doesn't exist |
| `ALREADY_EXISTS` | Overlay name already in use |
| `GIT_FAILED` | Git operation failed |
| `FUSE_NOT_FOUND` | macFUSE/FUSE-T not installed (macOS) |
| `PERMISSION_DENIED` | Insufficient permissions |
| `INVALID_CONFIG` | Configuration validation failed |
| `OVERLAY_NOT_MOUNTED` | Overlay exists but is not mounted |

## Development

### Build

```bash
make build          # Build for current platform -> dist/phantom
make build-all      # Build for all platforms -> dist/
make build-linux    # Build for Linux only
make build-darwin   # Build for macOS only
```

### Test

```bash
make test           # Run all tests
make test-short     # Run tests in short mode
make coverage       # Run tests with coverage report
```

### Quality Checks

```bash
make check          # Run fmt, vet, lint, and test
make fmt            # Format code
make vet            # Run go vet
make lint           # Run golangci-lint
```

### Release

```bash
make dist           # Build all platforms + generate checksums
make release        # Same as dist
```

### Run During Development

```bash
make dev            # Build and show help
make run ARGS='start /path/to/repo -n test'  # Build and run with args
```

## Makefile Targets

| Target | Description |
|--------|-------------|
| `build` | Build for current platform |
| `build-all` | Build for all platforms |
| `build-linux` | Build for Linux (amd64, arm64) |
| `build-darwin` | Build for macOS (amd64, arm64) |
| `build-platform` | Build for specific GOOS/GOARCH |
| `dist` | Build all + checksums |
| `release` | Create release |
| `clean` | Remove build artifacts |
| `install` | Install to /usr/local/bin |
| `uninstall` | Remove from /usr/local/bin |
| `test` | Run tests |
| `test-short` | Run tests (short mode) |
| `coverage` | Run tests with coverage |
| `check` | Run all quality checks |
| `fmt` | Format code |
| `vet` | Run go vet |
| `lint` | Run golangci-lint |
| `deps` | Download dependencies |
| `help` | Show help |

## License

MIT
