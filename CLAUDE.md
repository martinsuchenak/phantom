# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Phantom is a CLI tool for managing overlay filesystems that enables multiple AI agents to work on the same codebase in parallel without conflicts. Each agent gets its own isolated overlay with independent writes and its own git branch.

## Build and Development Commands

```bash
make build          # Build for current platform -> dist/phantom
make test           # Run all tests
make test-short     # Run tests in short mode
make coverage       # Tests with coverage report (coverage.html)
make check          # Run fmt + vet + lint + test (complete check)
make lint           # Run golangci-lint
make fmt            # Format code with gofmt
make run ARGS="start /path"  # Build and run with arguments
```

For cross-platform builds:
```bash
make build-all      # Cross-compile for linux/darwin (amd64/arm64)
make build-platform GOOS=linux GOARCH=arm64  # Specific platform
```

## Architecture

### Package Structure

```
cmd/main.go              # Entry point (version info, Execute())
internal/
  agent/                 # Agent process execution (env vars, logging, timeout)
  commands/              # All CLI commands (100+ files, one per command)
  config/                # Config loading (~/.phantom/config.yaml)
  git/                   # Git operations wrapper (branch, commit, push, merge)
  overlay/               # Platform-specific overlay implementations
  state/                 # Overlay state persistence with file locking
pkg/api/types.go         # Public types: Overlay, RunOptions, Error codes
```

### Core Abstractions

1. **Overlay Manager** (`internal/overlay/`) - Interface-based with platform-specific implementations via build tags (`//go:build linux`, `//go:build darwin`):
   - Linux: native overlayfs (root) or fuse-overlayfs (rootless)
   - macOS: unionfs-fuse via macFUSE or FUSE-T

2. **State Store** (`internal/state/`) - Persists overlay metadata as JSON in `~/.phantom/state/*.json` with file locking (`syscall.Flock`) for concurrent safety.

3. **Agent Runner** (`internal/agent/`) - Executes agent commands in overlay context, handles environment variables, logging, timeout, and exit codes.

### Command Pattern

Each CLI command follows this structure:
- `New*Command()` factory function creates the command
- `do*()` handler function contains the main logic
- `process*()` functions contain testable business logic

Command registration happens in `internal/commands/root.go`.

### Key Data Types

`pkg/api/types.go` defines:
- `Overlay` - Single overlay filesystem instance (name, paths, branch, etc.)
- `OverlayStatus` - Current status (mounted, size, uptime)
- `RunOptions` - Options for running agents (agent command, task, timeout, etc.)
- `OverlayError` - Structured errors with codes

## Platform-Specific Code

Use build tags for platform-specific implementations:
- `internal/overlay/linux.go` - `//go:build linux`
- `internal/overlay/darwin.go` - `//go:build darwin`

## Configuration

Config file: `~/.phantom/config.yaml`. Key settings:
- `state_dir` - Base directory for all phantom data
- `overlay.persistent` - Default persistence for new overlays
- `git.auto_branch` - Auto-create branch per overlay
- `agent.default_timeout_minutes` - Max agent execution time

See `docs/configuration.md` for full reference.

## Security Considerations

- State/config files use 0600 permissions, directories 0700
- Overlay names restricted to `[a-zA-Z0-9_-]`
- Git branch names validated against injection
- Agent commands parsed into args (not passed to shell)
- State files use file locking to prevent races

## Further Documentation

- `docs/commands.md` - Full command reference with all flags and options
- `docs/configuration.md` - Config file format, hooks, agent templates, agents.yaml
- `docs/workflows.md` - Real-world usage patterns (parallel race, map-reduce DAG, retry)
- `docs/tui.md` - Interactive TUI dashboard documentation
- `README.md` - Project overview, quick start, installation
