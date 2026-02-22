# Configuration

## Config File

Configuration is stored in `~/.phantom/config.yaml`. Generate defaults with `phantom init`.

```yaml
state_dir: "~/.phantom"

paths:
  overlays: ""                   # default: <state_dir>/overlays
  mounts: ""                     # default: <state_dir>/mnt
  logs: ""                       # default: <state_dir>/logs
  snapshots: ""                  # default: <state_dir>/snapshots

logging:
  level: info                    # trace, debug, info, warn, error, fatal
  file: "~/.phantom/phantom.log"

overlay:
  persistent: false              # default persistence for new overlays
  auto_cleanup_days: 7           # 0 to disable auto-cleanup

git:
  auto_branch: true              # auto-create branch per overlay
  branch_prefix: "phantom/"      # prefix for auto-generated branches
  auto_push_on_stop: false       # push branch when stopping overlay

darwin:
  unionfs_path: ""               # auto-detect unionfs-fuse
  fuse_options:
    - "cow"                      # copy-on-write mode

linux:
  use_fuse: false                # auto-detects if not root; true to force
  fuse_overlay_path: ""          # auto-detect fuse-overlayfs
  fuse_options: []

agent:
  default_timeout_minutes: 60    # max: 1440 (24 hours)
  cleanup_on_success: true       # cleanup overlay after successful agent run
  cleanup_on_failure: false      # cleanup overlay after failed agent run

agent_env:
  - "OVERLAY_ENABLED=true"       # extra env vars passed to agents
```

### Validation Rules

- `logging.level` must be one of: `trace`, `debug`, `info`, `warn`, `error`, `fatal`
- `agent.default_timeout_minutes` must be between 0 and 1440
- `overlay.auto_cleanup_days` cannot be negative (0 disables auto-cleanup)
- `state_dir` cannot be empty

Validate your config:

```bash
phantom config validate
phantom config show
```

---

## Auto-Cleanup

The `overlay.auto_cleanup_days` setting controls automatic removal of old overlays. When `phantom start` or `phantom run` executes, Phantom silently removes overlays that are:

- Older than the configured threshold
- Not currently mounted
- Not marked as persistent
- Not locked

Set to `0` to disable. For manual control, use `phantom prune`.

---

## Directory Structure

```
~/.phantom/
├── config.yaml              # Configuration (0600)
├── hooks.yaml               # Post-run hooks (0600)
├── phantom.log              # Application log
├── state/                   # Overlay state files (0600 each)
│   ├── feature-a.json
│   └── feature-b.json
├── overlays/                # Overlay data (0700)
│   ├── feature-a/
│   │   ├── upper/           # Writable layer
│   │   └── work/            # Work directory (Linux only)
│   └── feature-b/
├── mnt/                     # Mount points (0700)
│   ├── feature-a/
│   └── feature-b/
├── logs/                    # Agent execution logs (0700)
│   ├── feature-a.log
│   └── feature-b.log
└── snapshots/               # Overlay snapshots (0700)
    └── feature-a-20260220-143000/
        ├── meta.json
        └── data/
```

---

## Hooks

Hooks run automatically after any agent finishes in `phantom run`, `run-all`, or `run-chain`.

### Setup

```bash
# Create example hooks.yaml
phantom hook init

# Or add hooks individually
phantom hook add --name lint --on success --command "npm run lint --fix"
phantom hook add --name notify --on failure --command "echo failed >> ~/log.txt"
phantom hook add --name always-log --on always --command "echo done"
```

### hooks.yaml Format

```yaml
hooks:
  - name: lint
    on: success          # success | failure | always
    command: "npm run lint --fix 2>/dev/null || true"
  - name: test
    on: success
    command: "go test ./... 2>&1 | tail -5"
  - name: notify-failure
    on: failure
    command: "echo 'Agent failed in $OVERLAY_NAME' >> ~/.phantom/failures.log"
```

### Available Environment Variables

Hooks execute in the overlay mount point with these env vars:

| Variable | Description |
|----------|-------------|
| `OVERLAY_NAME` | Overlay name |
| `OVERLAY_PATH` | Mount point path |
| `OVERLAY_BASE` | Base directory |
| `OVERLAY_BRANCH` | Git branch |
| `OVERLAY_EXIT_CODE` | Agent exit code |
| `OVERLAY_AGENT` | Agent command that was run |
| `OVERLAY_TASK` | Task description |

### Webhook Examples

```bash
# Slack
phantom hook add --name slack --on failure \
  --command 'curl -s -X POST https://hooks.slack.com/services/XXX -d "{\"text\":\"$OVERLAY_AGENT failed in $OVERLAY_NAME\"}"'

# Discord
phantom hook add --name discord --on success \
  --command 'curl -s -X POST https://discord.com/api/webhooks/XXX -H "Content-Type: application/json" -d "{\"content\":\"✅ $OVERLAY_NAME completed\"}"'

# macOS notification
phantom hook add --name desktop --on always \
  --command 'osascript -e "display notification \"$OVERLAY_AGENT finished\" with title \"Phantom\""'
```

---

## Agent Templates

Built-in templates define the correct command and task delivery mode for popular AI agents.

```bash
phantom template list
```

| Template | Command | Task Mode |
|----------|---------|-----------|
| `claude` | `claude --print` | stdin |
| `claude-interactive` | `claude "{task}"` | placeholder |
| `gemini` | `gemini` | stdin |
| `gemini-arg` | `gemini "{task}"` | placeholder |
| `aider` | `aider --message "{task}"` | placeholder |
| `vibe` | `vibe --prompt "{task}"` | placeholder |
| `copilot` | `copilot --prompt "{task}" --allow-all-tools` | placeholder |
| `gh-copilot` | `gh copilot suggest "{task}"` | placeholder |
| `codex` | `codex "{task}"` | placeholder |
| `opencode` | `opencode run "{task}"` | placeholder |
| `opencode-stdin` | `opencode run --prompt` | stdin |
| `qwen-code` | `qwen --prompt "{task}"` | placeholder |
| `qwen-code-stdin` | `qwen` | stdin |
| `kiro` | `kiro chat --no-interactive --trust-all-tools` | stdin |

### Task Delivery Modes

- **stdin**: Task text is piped to the agent's stdin. Works when the agent reads from stdin in headless/non-TTY mode.
- **placeholder**: `{task}` in the agent command is replaced with the task text.
- **inline**: Embed the prompt directly in the agent command string.

### Generate Config from Templates

```bash
phantom template generate --agents claude,aider,gemini -o agents.yaml
```

Edit the `task` fields in the generated file, then run:

```bash
phantom run-all ~/myproject --config agents.yaml
```

---

## agents.yaml Format

Used by `phantom run-all`:

```yaml
# Parallel mode (default)
agents:
  - name: auth-agent
    agent: "claude --print"
    task: "implement authentication"
    branch: "feature/auth"
    timeout: 30
  - name: api-agent
    agent: 'aider --message "{task}"'
    task: "build REST API"
    branch: "feature/api"
```

```yaml
# Sequential mode
mode: sequential
name: pipeline
branch: feature/full
agents:
  - name: implement
    agent: "claude --print"
    task: "implement the feature"
  - name: test
    agent: "claude --print"
    task: "write tests"
```

## chain.yaml Format

Used by `phantom run-chain`:

```yaml
name: feature-pipeline
branch: feature/auth
steps:
  - name: implement
    agent: "claude --print"
    task: "implement authentication module"
    timeout: 30
  - name: test
    agent: "claude --print"
    task: "write unit tests"
  - name: lint
    agent: 'aider --message "{task}"'
    task: "fix linting errors"
```

---

## Environment Variables

Phantom loads `.env` files from the current directory automatically.

```bash
# .env
OVERLAY_CONFIG=/path/to/config.yaml
OVERLAY_AGENT=claude --print
OVERLAY_NAME=my-feature
OVERLAY_TASK=implement authentication
OVERLAY_BRANCH=feature/auth
```

| Variable | Maps to |
|----------|---------|
| `OVERLAY_CONFIG` | `--config` flag |
| `OVERLAY_NAME` | `--name` flag |
| `OVERLAY_AGENT` | `--agent` flag |
| `OVERLAY_TASK` | `--task` flag |
| `OVERLAY_BASE` | `base-dir` argument |
| `OVERLAY_BRANCH` | `--branch` flag |

The `.env` file supports comments (`#`), variable expansion (`$VAR`, `${VAR}`), and quoted values.

---

## Platform Notes

### Linux (Native overlayfs)

Requires root or appropriate capabilities:

```
mount -t overlay overlay \
  -o lowerdir=/base,upperdir=/upper,workdir=/work \
  /mnt/point
```

### Linux (fuse-overlayfs, rootless)

Auto-detected when not root and fuse-overlayfs is installed:

```
fuse-overlayfs -o lowerdir=/base,upperdir=/upper,workdir=/work /mnt/point
```

Force with `linux.use_fuse: true` in config.

### macOS

Requires macFUSE or FUSE-T + unionfs-fuse:

```
unionfs-fuse -o cow upperdir=RW:lowerdir=RO /mnt/point
```
