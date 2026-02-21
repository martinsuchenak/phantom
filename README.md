# Phantom

A CLI tool for managing overlay filesystems to enable multiple AI agents to work on the same codebase in parallel without conflicts. Each agent gets its own isolated overlay with independent writes and its own git branch.

## Features

- Cross-platform: Linux (native overlayfs + fuse-overlayfs) and macOS (unionfs-fuse)
- Git integration: automatic branch per overlay, commit, push, merge
- Run agents in parallel (`run-all`) or sequentially (`run-chain`)
- Post-run hooks for linting, testing, notifications
- Overlay snapshots, cloning, merging, comparing
- Lock/pin overlays to prevent accidental cleanup or base drift
- Single binary, no runtime dependencies beyond FUSE

## Quick Start

```bash
# Install
brew install martinsuchenak/tap/phantom

# Initialize config
phantom init

# Create an overlay and run an agent
phantom run ~/myproject --agent "claude --print" --task "implement auth" --name auth-feature

# Check what changed
phantom diff auth-feature

# Apply changes back to the base repo
phantom apply auth-feature --cleanup
```

### Run Multiple Agents in Parallel

```bash
# Three agents, same codebase, isolated overlays
phantom run-all ~/myproject --config agents.yaml --cleanup

# Or inline
phantom run-all ~/myproject --agents "claude --print,aider,gemini" --timeout 30
```

### Chain Agents Sequentially

```bash
# Each step builds on the previous one's work
phantom run-chain ~/myproject --steps "claude --print,aider" --name pipeline
```

## Documentation

- [Command Reference](docs/commands.md) — every command, flag, and option
- [Workflows & Examples](docs/workflows.md) — real-world usage patterns and recipes
- [Configuration](docs/configuration.md) — config file, hooks, templates, environment variables

## Installation

### Homebrew (macOS/Linux)

```bash
brew tap martinsuchenak/tap
brew install phantom
```

### Download Binary

Download from [GitHub Releases](https://github.com/martinsuchenak/phantom/releases).

```bash
curl -LO https://github.com/martinsuchenak/phantom/releases/latest/download/phantom_Linux_amd64.tar.gz
tar xzf phantom_Linux_amd64.tar.gz
sudo mv phantom /usr/local/bin/
```

### Build from Source

```bash
make build        # -> dist/phantom
make install      # -> /usr/local/bin/phantom
```

### Prerequisites

**Linux:** Native overlayfs (requires root) or fuse-overlayfs for rootless (`apt install fuse-overlayfs`). Auto-detected.

**macOS:** [macFUSE](https://osxfuse.github.io/) or [FUSE-T](https://github.com/macos-fuse-t/fuse-t) + [unionfs-fuse](https://github.com/rpodgorny/unionfs-fuse).

## Commands Overview

| Command | Description |
|---------|-------------|
| `start` / `stop` / `restart` | Create, unmount, remount overlays |
| `list` / `status` / `inspect` | View overlay state |
| `run` / `run-all` / `run-chain` | Run agents (single, parallel, sequential) |
| `diff` / `compare` / `conflicts` | View and compare changes |
| `commit` / `apply` / `merge` | Commit, apply to base, merge between overlays |
| `watch` / `logs` / `replay` | Monitor and re-run agents |
| `hook` | Post-run automation (lint, test, notify) |
| `snapshot` / `export` / `clone` | Save, export, duplicate overlays |
| `lock` / `unlock` / `pin` / `unpin` | Protect overlays from cleanup and base drift |
| `prune` / `gc` / `health` | Maintenance and diagnostics |
| `config` / `init` / `template` | Configuration and scaffolding |
| `completion` | Shell completion (bash, zsh, fish, powershell) |

See [docs/commands.md](docs/commands.md) for the full reference.

## How It Works

Phantom creates a writable overlay on top of your project directory. The base stays untouched — all writes go to an isolated upper layer. Each overlay gets its own mount point, git branch, and state.

```
Base Directory (read-only lower layer)
         ↓
   ┌─────────────┐
   │  Overlay A   │  → phantom/feature-auth  → ~/.phantom/mnt/feature-auth/
   │  Overlay B   │  → phantom/feature-api   → ~/.phantom/mnt/feature-api/
   │  Overlay C   │  → phantom/feature-tests → ~/.phantom/mnt/feature-tests/
   └─────────────┘
         ↑
  Isolated upper layers (~/.phantom/overlays/<name>/upper/)
```

**Linux:** Native kernel overlayfs (root) or fuse-overlayfs (rootless, auto-detected).
**macOS:** unionfs-fuse via macFUSE or FUSE-T.

## Development

```bash
make build          # Build for current platform
make test           # Run tests
make coverage       # Tests with coverage report
make check          # fmt + vet + lint + test
make build-all      # Cross-compile all platforms
```

## Security

- File permissions: state/config 0600, directories 0700
- Overlay names restricted to `[a-zA-Z0-9_-]`
- Git branch names validated against injection
- Agent commands parsed into args, not passed to shell
- State files use file locking to prevent races
- Symlink and path injection protection

## License

MIT
