# TUI Management Dashboard

`phantom manage` opens a full-screen interactive terminal UI for managing every aspect of your overlays without having to remember command flags.

```bash
phantom manage
phantom manage --theme amber
```

| Flag | Description |
|------|-------------|
| `--theme` | Color theme: `default`, `amber`, `blue`, `green`, `purple`, `light`, `plain` |

---

## Navigation

| Key | Action |
|-----|--------|
| `↑` / `↓` | Move between menu items |
| `Enter` | Select item / confirm prompt |
| `Esc` | Go back one level |
| `Ctrl+C` | Exit |

When a menu item requires input (e.g. a commit message or a name), a prompt appears inline. Type your answer and press `Enter` to confirm, or leave it blank and press `Enter` to cancel.

---

## Slash Commands

Type any slash command directly at the `CMD:` prompt at any time — you do not need to open the menu first.

| Command | Description |
|---------|-------------|
| `/menu` | Open the main management menu |
| `/start <base-dir> [name] [branch]` | Create and mount a new overlay |
| `/run <base-dir> <agent-cmd> [task…]` | Run a single agent |
| `/run-all <base-dir> <config.yaml>` | Run agents in parallel from a YAML config |
| `/run-chain <base-dir> <config.yaml>` | Run a chain of agents sequentially from a YAML config |
| `/health` | Run a system health check |
| `/health-fix` | Run health check and auto-fix issues |
| `/prune` | Prune unmounted overlays |
| `/prune-dry` | Preview what would be pruned (dry run) |
| `/gc` | Garbage-collect orphaned resources |
| `/gc-dry` | Preview what would be collected (dry run) |
| `/theme <name>` | Switch color theme at runtime |
| `/clear` | Clear the output pane |
| `/exit` | Exit the dashboard |

**Examples:**

```
/start ~/myproject
/start ~/myproject feature-a feat/auth
/run ~/myproject claude "implement the auth system"
/run-all ~/myproject agents.yaml
/run-chain ~/myproject chain.yaml
/theme purple
```

---

## Main Menu Structure

```
Phantom Management
├── Start New Overlay
│   ├── Quick start  (auto-name, auto-branch)        → prompt: base-dir
│   ├── Custom  (base-dir [name] [branch])            → prompt: all on one line
│   └── Persistent  (base-dir [name] [branch])        → prompt: all on one line
│
├── Run Agents
│   ├── Run  (single agent)                           → prompt: base-dir agent-cmd [task]
│   ├── Run  (guided)                                 → multi-step prompts
│   ├── Run-All  (parallel agents from config)        → prompt: base-dir config.yaml
│   └── Run-Chain  (sequential steps from config)     → prompt: base-dir config.yaml
│
├── Overlays  (N)
│   └── <overlay-name>  [status]
│       ├── Inspect
│       ├── View Logs  (last 4 KB)
│       ├── Replay  (re-run last agent command)
│       ├── Watch  (stream file changes)
│       ├── Mount
│       ├── Unmount
│       ├── Restart
│       ├── Sync
│       ├── Diff
│       ├── Commit
│       ├── Apply
│       ├── Merge
│       ├── Compare
│       ├── Export
│       ├── Clone
│       ├── Rename
│       ├── Pin / Unpin
│       ├── Lock / Unlock
│       ├── Stop
│       └── Cleanup
│
├── System Health
│   ├── Health check
│   └── Health check + auto-fix
│
├── Prune
│   ├── Dry run  (preview)
│   └── Prune unmounted overlays        → requires: yes
│
├── Garbage Collect
│   ├── Dry run  (preview)
│   └── Run GC                          → requires: yes
│
├── Theme
│   └── default / amber / blue / green / purple / light / plain
│
└── Exit
```

---

## Overlay Status Indicators

The overlay list shows a status tag next to each name:

| Tag | Meaning |
|-----|---------|
| `mounted` | FUSE mount is active |
| `unmounted` | Overlay exists but is not mounted |
| `🔒` | Locked — protected from cleanup, prune, and GC |
| `📌` | Persistent — survives prune |
| `📍` | Pinned to a specific base commit |

---

## Per-Overlay Actions

### Inspect

Displays a summary block:

```
Name:         my-feature
Branch:       phantom/my-feature
Status:       mounted
Mount point:  /tmp/phantom/mnt/my-feature
Base dir:     /home/user/myproject
Upper dir:    /tmp/phantom/overlays/my-feature/upper
Created:      2026-02-23 11:00:00
Locked:       false
Persistent:   false
Pinned:       (none)
Changes:      +5 added  ~3 modified  -1 deleted
```

### View Logs

Shows the last 4 KB of the overlay's agent log file. Useful for checking what an agent did or why it failed.

### Replay

Re-runs the last agent command recorded in the overlay's log file.

- **Dry run** — shows which agent and task would be replayed without executing.
- **Replay** — executes the agent again in the same overlay; requires `yes` confirmation.

### Watch

Streams file-change events from the overlay's upper directory in real time. The watcher runs in the background; it stops when you exit the dashboard or the context is cancelled.

### Mount / Unmount / Restart

| Action | Effect |
|--------|--------|
| **Mount** | Re-activate the FUSE mount (safe on already-mounted overlays) |
| **Unmount** | Unmount the FUSE volume; data is preserved in the upper directory |
| **Restart** | Unmount then immediately remount — useful after base changes |

### Sync

Pull changes from the base directory into a running overlay, keeping your overlay's own changes intact.

| Sub-option | Behaviour |
|------------|-----------|
| Dry run | Preview what would be rebased / remounted; no changes made |
| Sync | Perform the rebase (git repos) or remount (non-git dirs) |
| Sync + stash | Stash uncommitted changes first, then sync, then pop the stash |

### Diff

| Sub-option | Output |
|------------|--------|
| Stat only | Shows counts of added / modified / deleted files |
| Full diff | Shows a full file-by-file diff against the base |

### Commit

Prompts for a commit message and commits all staged and unstaged changes in the overlay mount point. The overlay must be mounted and the base directory must be a git repository.

### Apply

Merge the overlay's changes back into the base directory.

| Sub-option | Behaviour |
|------------|-----------|
| Dry run | Preview which files would be written; no changes made |
| Apply (keep running) | Copy changes to base; overlay stays mounted |
| Apply + Stop | Copy changes to base; unmount the overlay |
| Apply + Cleanup | Copy changes to base; unmount and delete all overlay data |

Apply + Cleanup requires `yes` confirmation.

### Merge

Copy file changes from a **source** overlay into **this** overlay. Useful for pulling a colleague's (or another agent's) work in.

| Sub-option | Behaviour |
|------------|-----------|
| Dry run | Preview which files would be copied / deleted |
| Merge | Copy changes; abort if there are conflicts |
| Force merge | Copy changes, overwriting conflicting files |

Each option prompts for the **source overlay name**.

### Compare

Side-by-side view of what changed in this overlay vs. another overlay:

```
FILE                                     my-feature    other-feature    NOTE
src/auth.go                              modified      modified         ⚠ diverged
src/utils.go                             added         —
tests/auth_test.go                       —             added

Only in my-feature: 1 | Only in other-feature: 1 | Both: 1
```

Prompts for the other overlay's name.

### Export

Save the overlay's changes to a file on disk.

| Format | Extension | Notes |
|--------|-----------|-------|
| Unified diff | `.patch` | Applies cleanly with `git apply` or `patch` |
| Tarball | `.tar.gz` | Contains all changed files at their paths |

Each option prompts for the output file path.

### Clone

Duplicate this overlay to a new name, copying all upper-directory changes. The new overlay is created from the same base directory. Prompts for the new overlay name.

### Rename

Rename this overlay (directories, mount points, and log files are all moved). The overlay **must be stopped** first. Prompts for the new name.

### Pin / Unpin

| Action | Effect |
|--------|--------|
| **Pin** | Records the current HEAD commit of the base directory. Future syncs will warn if the base has moved past this point. |
| **Unpin** | Removes the pin; syncs proceed freely. |

A pinned overlay shows `📍` in the overlay list.

### Lock / Unlock

| Action | Effect |
|--------|--------|
| **Lock** | Marks the overlay as locked — it will be skipped by `prune`, `gc`, and the TUI Cleanup action. |
| **Unlock** | Removes the lock. |

A locked overlay shows `🔒` in the overlay list.

### Stop

Unmounts the FUSE volume and clears the PID field in the state store. All data in the upper directory is preserved. The overlay can be remounted later with **Mount** or `phantom start`.

### Cleanup

Unmounts the FUSE volume and **permanently deletes** all overlay data (upper directory, mount point, log file). Requires typing `yes` to confirm.

---

## Running Agents from the TUI

Agent runs execute in a background goroutine so the dashboard remains fully interactive. All `log` output (info, warnings, errors) from the agent process is piped into the TUI output pane in real time. A spinner shows for the duration of the run.

### Run (single agent) — guided walkthrough

1. `/menu` → **Run Agents** → **Run (guided)**
2. **Base directory** → enter the repo path, e.g. `~/myproject`
3. **Agent command** → enter the agent binary or command, e.g. `claude`
4. Choose a sub-option:
   - **Run now** — no task, no cleanup
   - **Run with task** — enter a task description
   - **Run with task + cleanup** — enter a task description; overlay is deleted after the run

### Run-All (parallel agents)

Provide a standard `agents.yaml` config (same format as `phantom run-all --config`):

```
/run-all ~/myproject agents.yaml
```

Or via `/menu` → **Run Agents** → **Run-All (parallel agents from config)**.

### Run-Chain (sequential steps)

Provide a standard `chain.yaml` config (same format as `phantom run-chain --config`):

```
/run-chain ~/myproject chain.yaml
```

Or via `/menu` → **Run Agents** → **Run-Chain (sequential steps from config)**.

---

## System Health

Checks every overlay for common problems:

| Issue | Description |
|-------|-------------|
| `missing_base` | Base directory no longer exists on disk |
| `missing_upper` | Upper (writable) directory was deleted |
| `stale_mount` | Overlay is not mounted but is recorded as active |
| `dead_pid` | FUSE process PID is recorded but the process is not running |

**Health check + auto-fix** will attempt to remount any overlay flagged as `stale_mount`.

---

## Prune

Removes unmounted, non-persistent, and unlocked overlays in bulk.

- **Dry run** — lists candidates without deleting anything.
- **Prune** — deletes after `yes` confirmation. Skips overlays that are mounted, locked, or persistent.

---

## Garbage Collect

Cleans up orphaned filesystem resources that are no longer tracked in the state store:

- Overlay data directories
- Mount point directories
- Log files
- Snapshot directories

Anything not associated with a known overlay name is considered orphaned.

- **Dry run** — counts orphaned resources without removing them.
- **Run GC** — removes them after `yes` confirmation.

---

## Themes

Switch appearance at runtime with `/theme <name>` or via the **Theme** menu.

| Theme | Description |
|-------|-------------|
| `default` | Balanced dark theme |
| `amber` | Warm amber tones |
| `blue` | Cool blue tones |
| `green` | Terminal-green aesthetic |
| `purple` | Deep purple tones |
| `light` | Light background |
| `plain` | Minimal, no color |

The `--theme` flag sets the initial theme at startup:

```bash
phantom manage --theme amber
```
