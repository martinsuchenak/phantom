# Command Reference

Global flags available on all commands:

- `--config, -c` — Config file path (default: `~/.phantom/config.yaml`)
- `--verbose, -V` — Verbose output
- `--version, -v` — Print version information
- `--help, -h` — Show help for a command

---

## Overlay Lifecycle

### `phantom start <base-dir>`

Create and mount a new overlay filesystem.

```bash
phantom start /path/to/repo -n feature-a
phantom start ~/myproject -b feature/auth --persistent
```

| Flag | Description |
|------|-------------|
| `--name, -n` | Overlay name (default: base dir name) |
| `--branch, -b` | Git branch name (default: `phantom/<name>`) |
| `--persistent, -p` | Keep overlay data across reboots |

Prints the mount point path to stdout. Capture it in scripts:

```bash
PHANTOM_PATH=$(phantom start ~/myproject -n my-feature)
cd "$PHANTOM_PATH"
```

### `phantom stop <name>`

Unmount and optionally cleanup an overlay.

```bash
phantom stop feature-a
phantom stop feature-a --cleanup
phantom stop feature-a --push --force
```

| Flag | Description |
|------|-------------|
| `--cleanup` | Remove overlay data after unmounting |
| `--push` | Push branch to remote before stopping |
| `--force, -f` | Force unmount if stuck |

Locked overlays cannot be cleaned up unless `--force` is used.

### `phantom restart <name>`

Remount an overlay that was unmounted (e.g. after reboot). No-op if already mounted.

```bash
phantom restart feature-a
```

### `phantom list`

List overlays.

```bash
phantom list
phantom list --all
phantom list --format json
```

| Flag | Description |
|------|-------------|
| `--all` | Include unmounted overlays |
| `--format` | Output format: `table`, `json` |

### `phantom status [<name>]`

Show overlay state. Without a name, shows all overlays. Single-overlay view includes file change counts.

```bash
phantom status
phantom status feature-a
phantom status feature-a --format json
```

---

## Running Agents

### `phantom run <base-dir>`

Run a single agent in an overlay context.

```bash
phantom run ~/myproject --agent "claude --print --dangerously-skip-permissions --model {model}" --task "implement auth"
phantom run ~/myproject -a "aider --yes-always --model {model} --message '{task}'" -m gpt-4o
phantom run ~/myproject -a "aider" -t "fix tests" -n fix-tests --timeout 30 --cleanup
```

| Flag | Description |
|------|-------------|
| `--agent, -a` | Agent command to run (required) |
| `--task, -t` | Task description (passed as `OVERLAY_TASK` env var) |
| `--model, -m` | Model name — substituted as `{model}` in the agent command; also set as `OVERLAY_MODEL`. Optional. |
| `--name, -n` | Overlay name (auto-generated if omitted) |
| `--branch, -b` | Git branch name |
| `--timeout` | Timeout in minutes (default: from config, max: 1440) |
| `--cleanup` | Cleanup overlay after completion |
| `--push` | Push branch to remote on completion |
| `--persist, -p` | Keep overlay mounted after completion |

Environment variables set for the agent:

| Variable | Description |
|----------|-------------|
| `OVERLAY_NAME` | Overlay name |
| `OVERLAY_PATH` | Mount point path |
| `OVERLAY_BASE` | Original base directory |
| `OVERLAY_BRANCH` | Git branch name |
| `OVERLAY_TASK` | Task description |
| `OVERLAY_MODEL` | Model name (empty string if `--model` was not passed) |
| `OVERLAY_ENABLED` | Always `true` |

### `phantom run-all <base-dir>`

Run multiple agents in parallel, each in its own overlay.

```bash
phantom run-all ~/myproject --config agents.yaml
phantom run-all ~/myproject --agents "claude --print,aider,gemini"
phantom run-all ~/myproject --config agents.yaml --model claude-opus-4-5 --timeout 30 --cleanup
```

| Flag | Description |
|------|-------------|
| `--config, -c` | Path to agents YAML config file |
| `--agents` | Comma-separated agent commands (simple mode) |
| `--model, -m` | Model name — overrides `model:` in YAML for all agents. Optional. |
| `--timeout` | Global timeout per agent in minutes |
| `--cleanup` | Cleanup all overlays after completion |
| `--push` | Push branches to remote |
| `--format` | Summary format: `table` (default), `json` |

Config file supports `mode: sequential` to run agents on a single overlay instead of parallel (see [workflows](workflows.md#sequential-mode-in-run-all)).

### `phantom run-chain <base-dir>`

Run agents sequentially on a single overlay. Each step builds on the previous one's work.

```bash
phantom run-chain ~/myproject --config chain.yaml
phantom run-chain ~/myproject --steps "claude --print,aider,gemini"
phantom run-chain ~/myproject --config chain.yaml --model claude-sonnet-4 --continue-on-error --cleanup
```

| Flag | Description |
|------|-------------|
| `--config, -c` | Path to chain YAML config file |
| `--steps` | Comma-separated agent commands (simple mode) |
| `--name, -n` | Overlay name (auto-generated if omitted) |
| `--branch, -b` | Git branch name |
| `--model, -m` | Model name — overrides `model:` in YAML for all steps. Optional. |
| `--timeout` | Global timeout per step in minutes |
| `--cleanup` | Cleanup overlay after completion |
| `--push` | Push branch to remote on completion |
| `--continue-on-error` | Keep running remaining steps on failure |
| `--format` | Summary format: `table` (default), `json` |

### `phantom replay <name>`

Re-run the last agent command that was executed in an overlay. Reads from the log file header.

```bash
phantom replay feature-a
phantom replay feature-a --dry-run
phantom replay feature-a --timeout 30 --cleanup
```

| Flag | Description |
|------|-------------|
| `--dry-run` | Show what would be run without executing |
| `--timeout` | Timeout in minutes |
| `--cleanup` | Cleanup overlay after completion |
| `--push` | Push branch to remote on completion |

---

## Viewing Changes

### `phantom diff <name>`

Show files changed in an overlay.

```bash
phantom diff feature-a
phantom diff feature-a --stat
phantom diff feature-a --format json
phantom diff feature-a --format simple
```

| Flag | Description |
|------|-------------|
| `--format` | Output format: `table` (default), `json`, `simple` |
| `--stat` | Summary statistics only |

### `phantom compare <overlay-a> <overlay-b>`

Side-by-side comparison of what two overlays changed.

```bash
phantom compare feature-auth feature-api
phantom compare feature-auth feature-api --format json
```

Files changed in both overlays are flagged as "identical change" or "⚠ diverged".

### `phantom conflicts "<names>"`

Detect file conflicts between two or more overlays.

```bash
phantom conflicts "feature-a feature-b"
phantom conflicts "auth-agent api-agent test-agent" --format json
```

Reports which files were changed by more than one overlay.

### `phantom watch <name>`

Stream file changes in real time.

```bash
phantom watch feature-a
phantom watch feature-a --interval 1
phantom watch feature-a --format json
```

| Flag | Description |
|------|-------------|
| `--interval, -i` | Poll interval in seconds (default: 2) |
| `--format` | Output format: `simple` (default), `json` |

Output symbols: `+` added, `~` modified, `-` deleted, `⊘` reset. Press Ctrl+C to stop.

### `phantom logs <name>`

View agent execution logs.

```bash
phantom logs feature-a
phantom logs feature-a --tail 4096
phantom logs feature-a --path
```

| Flag | Description |
|------|-------------|
| `--tail, -n` | Show last N bytes (0 = entire file) |
| `--path` | Print log file path instead of contents |

### `phantom inspect <name>`

Full debug view: state, mount status, FUSE PID, git info, file changes, snapshots, lock/pin status, log path.

```bash
phantom inspect feature-a
phantom inspect feature-a --format json
```

---

## Applying Changes

### `phantom commit <name>`

Commit all changes in an overlay.

```bash
phantom commit feature-a -m "implement auth module"
phantom commit feature-a -m "implement auth" --push
```

| Flag | Description |
|------|-------------|
| `--message, -m` | Commit message (required) |
| `--push` | Push to remote after committing |

### `phantom apply <name>`

Apply overlay changes to the base directory.

```bash
phantom apply feature-a
phantom apply feature-a --dry-run
phantom apply feature-a --stop --cleanup
```

| Flag | Description |
|------|-------------|
| `--dry-run` | Preview without making changes |
| `--stop` | Stop overlay after applying |
| `--cleanup` | Cleanup overlay data (implies `--stop`) |

For git repos: auto-commits, then merges overlay branch into base. For non-git: copies changed files, removes deleted ones.

### `phantom merge <source> <target>`

Merge changes from one overlay into another.

```bash
phantom merge feature-auth feature-combined
phantom merge feature-auth feature-combined --dry-run
phantom merge feature-auth feature-combined --force
```

| Flag | Description |
|------|-------------|
| `--dry-run` | Show merge plan without making changes |
| `--force, -f` | Overwrite conflicting files in target |

### `phantom sync <name>`

Pull latest base directory changes into a running overlay.

```bash
phantom sync feature-a
phantom sync feature-a --dry-run
phantom sync feature-a --stash
```

| Flag | Description |
|------|-------------|
| `--dry-run` | Preview without making changes |
| `--stash` | Stash uncommitted changes before sync, restore after |

Git repos: fetches and rebases. Non-git: remounts to refresh base layer. Warns if overlay is pinned and base has diverged.

---

## Protection

### `phantom lock <name>` / `phantom unlock <name>`

Toggle cleanup protection on an overlay.

```bash
phantom lock feature-a
phantom unlock feature-a
```

Locked overlays are protected from `stop --cleanup`, `prune`, `gc`, and auto-cleanup. Unlike `--persistent` (set at creation), lock/unlock toggles at runtime.

### `phantom pin <name>` / `phantom unpin <name>`

Pin an overlay to a specific base commit.

```bash
phantom pin feature-a
phantom pin feature-a --commit abc123
phantom unpin feature-a
```

| Flag | Description |
|------|-------------|
| `--commit` | Pin to specific commit (default: current HEAD) |

When pinned, `sync` warns if the base has diverged. Pin status shown in `inspect`.

---

## Snapshots & Export

### `phantom snapshot`

Save, restore, list, and delete overlay snapshots.

```bash
phantom snapshot save feature-a
phantom snapshot save feature-a -s "before-refactor"
phantom snapshot list
phantom snapshot list feature-a --format json
phantom snapshot restore feature-a feature-a-20260220-143000
phantom snapshot delete feature-a-20260220-143000
```

Overlay must be unmounted before restoring.

### `phantom export <name>`

Export overlay changes as a diff or tarball.

```bash
phantom export feature-a
phantom export feature-a -o changes.patch
phantom export feature-a --format tar -o changes.tar.gz
```

| Flag | Description |
|------|-------------|
| `--output, -o` | Output file path |
| `--format` | Export format: `diff` (default), `tar` |

### `phantom clone <source> <destination>`

Clone an overlay with all its changes.

```bash
phantom clone feature-a feature-a-experiment
phantom clone feature-a feature-a-v2 --branch feature/v2
```

---

## Automation

### `phantom hook`

Manage post-run hooks that fire automatically after agent completion.

```bash
phantom hook list
phantom hook add --name lint --on success --command "npm run lint --fix"
phantom hook add --name notify --on failure --command "echo 'Failed: $OVERLAY_NAME'"
phantom hook add --name format --on always --command "go fmt ./..."
phantom hook remove lint
phantom hook init
```

Trigger conditions: `success`, `failure`, `always`. Hooks execute in the overlay mount point with env vars: `OVERLAY_NAME`, `OVERLAY_PATH`, `OVERLAY_BASE`, `OVERLAY_BRANCH`, `OVERLAY_EXIT_CODE`, `OVERLAY_AGENT`, `OVERLAY_TASK`.

Stored in `~/.phantom/hooks.yaml`.

---

## Maintenance

### `phantom prune`

Remove stale and expired overlays.

```bash
phantom prune
phantom prune --dry-run
phantom prune --force
```

Skips persistent and locked overlays. `--force` also removes mounted but expired overlays.

### `phantom gc`

Garbage collect orphaned resources (data dirs, mount points, logs, broken snapshots).

```bash
phantom gc
phantom gc --dry-run
```

### `phantom health`

Check system and overlay health.

```bash
phantom health
phantom health --fix
phantom health --format json
```

`--fix` attempts to remount stale overlays and clean zombies.

### `phantom rename <old> <new>`

Rename an overlay (must be stopped first).

```bash
phantom stop feature-a
phantom rename feature-a feature-auth
phantom restart feature-auth
```

---

## Configuration & Setup

### `phantom init`

Create default config and example agents.yaml.

```bash
phantom init
phantom init --output /path/to/dir --force
```

### `phantom config`

```bash
phantom config validate
phantom config validate --path /path/to/config.yaml
phantom config show
```

### `phantom template`

Browse and generate config files from built-in agent templates.

```bash
# List all templates
phantom template list

# Show details and example YAML for a template
phantom template show claude

# Generate agents.yaml for run-all (parallel)
phantom template generate --agents claude,aider,gemini -o agents.yaml

# Generate chain.yaml for run-chain (sequential pipeline)
phantom template generate --chain --agents claude,aider,gemini -o chain.yaml

# Generate chain.yaml with pre-filled name and branch
phantom template generate --chain --agents claude,aider --name auth-pipeline --branch feature/auth -o chain.yaml
```

| Subcommand | Description |
|------------|-------------|
| `list` | List all built-in templates with agent command and task mode |
| `show <name>` | Show template details and an example agents.yaml entry |
| `generate` | Generate a config file from one or more templates |

**`generate` flags:**

| Flag | Description |
|------|-------------|
| `--agents` | Comma-separated template names, e.g. `claude,aider,gemini` (required) |
| `--chain, -c` | Output `chain.yaml` format for `run-chain` (default: `agents.yaml` for `run-all`) |
| `--name, -n` | Pipeline name (chain mode only, default: `my-pipeline`) |
| `--branch, -b` | Git branch (chain mode only) |
| `--output, -o` | Write to file instead of stdout |

Built-in templates: `claude`, `claude-interactive`, `gemini`, `gemini-arg`, `aider`, `vibe`, `copilot`, `gh-copilot`, `codex`, `opencode`, `opencode-stdin`, `qwen-code`, `qwen-code-stdin`, `kiro`.

### `phantom completion <shell>`

Generate shell completion scripts. Supported shells: `bash`, `zsh`, `fish`, `powershell`.

**Bash** — load for current session:

```bash
source <(phantom completion bash)
```

Load permanently (execute once):

```bash
phantom completion bash > ~/.bash_completion
```

**Zsh** — load for current session:

```zsh
source <(phantom completion zsh)
```

Load permanently via fpath (execute once, then restart your shell):

```zsh
phantom completion zsh > "${fpath[1]}/_phantom"
```

If completions still don't appear, ensure your `~/.zshrc` contains:

```zsh
autoload -U compinit
compinit
```

**Fish**:

```fish
phantom completion fish > ~/.config/fish/completions/phantom.fish
```

**PowerShell**:

```powershell
phantom completion powershell | Out-String | Invoke-Expression
```

---

## Error Codes

| Code | Description |
|------|-------------|
| `MOUNT_FAILED` | Failed to mount overlay |
| `UNMOUNT_FAILED` | Failed to unmount |
| `NOT_FOUND` | Overlay doesn't exist |
| `ALREADY_EXISTS` | Overlay name already in use |
| `GIT_FAILED` | Git operation failed |
| `FUSE_NOT_FOUND` | macFUSE/FUSE-T not installed |
| `PERMISSION_DENIED` | Insufficient permissions |
| `INVALID_CONFIG` | Configuration validation failed |
| `OVERLAY_NOT_MOUNTED` | Overlay exists but is not mounted |
| `OVERLAY_LOCKED` | Overlay is locked and cannot be cleaned up |
