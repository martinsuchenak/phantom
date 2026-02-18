# Phantom

A Go-based CLI tool for managing overlay filesystems to enable multiple AI agents to work on the same codebase in parallel without conflicts. Each agent gets its own phantom overlay with isolated writes and its own git branch.

## Features

- **Cross-platform**: Linux (native overlayfs) + macOS (unionfs-fuse)
- **Git integration**: automatic branch per overlay
- **Optional persistence** across reboots
- **Agent wrapper** for running commands in overlay context
- **Single binary** distribution
- **Secure by default**: restrictive file permissions, input validation

## Installation

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

### Install

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
- unionfs-fuse: `brew install unionfs-fuse`

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
# Status of specific overlay
phantom status feature-a

# Status of all overlays
phantom status

# Output as JSON
phantom status feature-a --format json
```

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

## Commands

| Command | Description |
|---------|-------------|
| `phantom start <base-dir>` | Create overlay, print mount path |
| `phantom stop <name>` | Unmount and optionally cleanup/push |
| `phantom list` | List all active overlays |
| `phantom status [<name>]` | Show overlay state |
| `phantom run <base-dir> --agent <cmd>` | Run agent in overlay context |

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
- `overlay.auto_cleanup_days` cannot be negative
- `state_dir` cannot be empty

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
└── mnt/                 # Mount points (0700 permissions)
    ├── feature-a/
    └── feature-b/
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
