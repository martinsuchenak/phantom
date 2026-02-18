# Phantom

A Go-based CLI tool for managing overlay filesystems to enable multiple AI agents to work on the same codebase in parallel without conflicts. Each agent gets its own phantom overlay with isolated writes and its own git branch.

## Features

- **Cross-platform**: Linux (native overlayfs) + macOS (unionfs-fuse)
- **Git integration**: automatic branch per overlay
- **Optional persistence** across reboots
- **Agent wrapper** for running commands in overlay context
- **Single binary** distribution

## Installation

### Build from Source

```bash
go build -o phantom ./cmd
```

Or use the Makefile:

```bash
make build
```

### Prerequisites

**Linux:**
- Kernel with overlayfs support (most modern kernels)

**macOS:**
- [macFUSE](https://osxfuse.github.io/) or [FUSE-T](https://github.com/macos-fuse-t/fuse-t)
- unionfs-fuse: `brew install unionfs-fuse`

## Usage

### Create a Phantom Overlay

```bash
# Create an overlay for a directory
./phantom start /path/to/repo -n feature-a

# Output: /Users/user/.phantom/mnt/feature-a
```

The overlay mount point is printed to stdout, making it easy to capture in scripts:

```bash
PHANTOM_PATH=$(./phantom start ~/myproject -n my-feature)
cd "$PHANTOM_PATH"
# Work in the isolated overlay...
```

### List Overlays

```bash
./phantom list
```

### Show Status

```bash
./phantom status feature-a
```

### Stop an Overlay

```bash
# Unmount only
./phantom stop feature-a

# Unmount and cleanup all data
./phantom stop feature-a --cleanup

# Push changes before stopping
./phantom stop feature-a --push
```

### Run an Agent in Overlay Context

```bash
./phantom run --agent "claude code" --task "implement auth" --base /path/to/repo
```

This sets environment variables for the agent:
- `OVERLAY_NAME` - Overlay name
- `OVERLAY_PATH` - Mount point
- `OVERLAY_BASE` - Original directory
- `OVERLAY_BRANCH` - Git branch
- `OVERLAY_TASK` - Task description

## Commands

| Command | Description |
|---------|-------------|
| `phantom start <base-dir>` | Create overlay, print mount path |
| `phantom stop <name>` | Unmount and optionally cleanup/push |
| `phantom list` | List all active overlays |
| `phantom status [<name>]` | Show overlay state |
| `phantom run --agent <cmd> --task <desc>` | Run agent in overlay context |

### Global Flags

- `--config, -c` - Config file path (default: `~/.phantom/config.yaml`)
- `--verbose, -v` - Verbose output

## Configuration

Configuration is stored in `~/.phantom/config.yaml`:

```yaml
state_dir: "~/.phantom"
logging:
  level: info
  file: "~/.phantom/phantom.log"
overlay:
  persistent: false
  auto_cleanup_days: 7
git:
  auto_branch: true
  branch_prefix: "phantom/"
  auto_push_on_stop: false
darwin:
  unionfs_path: ""  # auto-detect
  fuse_options:
    - "cow"
agent:
  default_timeout_minutes: 60
  cleanup_on_success: true
  cleanup_on_failure: false
agent_env:
  OVERLAY_ENABLED: "true"
```

## How It Works

### Linux

Uses native kernel overlayfs via syscall:

```
mount -t overlay overlay \
  -o lowerdir=/base,upperdir=/upper,workdir=/work \
  /mnt/point
```

### macOS

Uses unionfs-fuse (requires macFUSE or FUSE-T):

```
unionfs-fuse -o cow upperdir=RW:lowerdir=RO /mnt/point
```

## Directory Structure

```
~/.phantom/
├── config.yaml          # Configuration
├── phantom.log          # Log file
├── state/               # Overlay state files
│   ├── feature-a.json
│   └── feature-b.json
├── overlays/            # Overlay data
│   ├── feature-a/
│   │   ├── upper/       # Writable layer
│   │   └── work/        # Work directory (Linux only)
│   └── feature-b/
└── mnt/                 # Mount points
    ├── feature-a/
    └── feature-b/
```

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

## Development

### Build

```bash
make build
```

### Build for All Platforms

```bash
make build-all
```

### Test

```bash
make test
```

## License

MIT
