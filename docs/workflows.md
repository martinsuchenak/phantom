# Workflows & Examples

Real-world usage patterns for Phantom. Each workflow shows a complete sequence of commands for a specific use case.

---

## Parallel Agent Race

Run multiple agents on the same task, compare results, keep the best one.

```bash
# 1. Run three agents in parallel
phantom run-all ~/myproject --config agents.yaml --timeout 30

# 2. Check what each agent changed
phantom diff auth-agent
phantom diff api-agent
phantom diff test-agent

# 3. Compare two candidates side by side
phantom compare auth-agent api-agent

# 4. Check for conflicts before merging
phantom conflicts "auth-agent api-agent"

# 5. Apply the best result to the base repo
phantom apply auth-agent --cleanup

# 6. Clean up the rest
phantom stop api-agent --cleanup
phantom stop test-agent --cleanup
```

Example `agents.yaml`:

```yaml
agents:
  - name: auth-agent
    agent: "claude --print"
    task: "implement JWT authentication with refresh tokens"
    branch: "feature/auth-claude"
  - name: api-agent
    agent: 'aider --message "{task}"'
    task: "implement JWT authentication with refresh tokens"
    branch: "feature/auth-aider"
  - name: test-agent
    agent: "gemini"
    task: "implement JWT authentication with refresh tokens"
    branch: "feature/auth-gemini"
```

---

## Map-Reduce Pipeline (DAG)

Construct a Directed Acyclic Graph (DAG) for complex operations that involve parallel sub-tasks feeding into a sequencer.

```bash
phantom run-pipeline ~/myproject --config map-reduce.yaml --cleanup
```

`map-reduce.yaml`:

```yaml
name: map-reduce-feature
agents:
  - name: frontend-agent
    agent: "claude --print"
    task: "build the React component for the new dashboard"
  - name: backend-agent
    agent: "claude --print"
    task: "build the Go API handlers for the new dashboard"
  - name: e2e-tests
    agent: "aider"
    task: "write Cypress end-to-end tests for the new dashboard"
    depends_on:
      - frontend-agent
      - backend-agent
```

Independent agents `frontend-agent` and `backend-agent` will run in parallel. When *both* complete successfully, `e2e-tests` will execute, first automatically fetching and merging the branches of its parents. If merge conflicts occur between parallel agents, the sequencer agent is prompted to resolve them automatically!

**Note:** `run-pipeline` requires a completely clean Git working tree before starting. Ensure all tracking files are committed or stashed.

---

## Resuming / Retrying Jobs

If a pipeline or `run-all` execution fails, stops prematurely, or you wish to retry specific parts from where you left off, use `--from` / `--only` combined with reusing names. Phantom will automatically load and mount your existing overlays and branches, satisfying dependencies.

```bash
# 1. Pipeline failed while running 'e2e-tests', but 'frontend-agent' and 'backend-agent' were successful.
# Provide the pipeline --name exactly as it appears in status so Phantom knows which overlays to reuse:
phantom run-pipeline ~/myproject --config map-reduce.yaml --name map-reduce-feature --from e2e-tests
```

When resuming with `--from` or `--only`:
- Preceding or unselected agents will be **skipped**. By retaining the `--name` of your execution, Phantom finds the skipped agents' existing overlays and passes their branches to downstream agents.
- **Git fetching/merging** dependencies will still work instantly.

---

## Pipeline: Implement → Test → Lint

Chain agents so each builds on the previous one's work.

```bash
phantom run-chain ~/myproject --config pipeline.yaml --name auth-pipeline --push
```

`pipeline.yaml`:

```yaml
name: auth-pipeline
branch: feature/auth-complete
steps:
  - name: implement
    agent: "claude --print"
    task: "implement the authentication module with JWT tokens, middleware, and user model"
    timeout: 30
  - name: test
    agent: "claude --print"
    task: "write comprehensive unit tests for the authentication module, aim for 90% coverage"
    timeout: 20
  - name: lint-fix
    agent: 'aider --message "{task}"'
    task: "fix all linting errors and format the code according to project standards"
    timeout: 10
```

The chain stops on first failure. Use `--continue-on-error` to run all steps regardless.

---

## Sequential Mode in run-all

Instead of a separate `run-chain` command, you can use `mode: sequential` in your existing `agents.yaml`:

```yaml
mode: sequential
name: feature-pipeline
branch: feature/full-stack
agents:
  - name: implement
    agent: "claude --print"
    task: "implement the feature"
  - name: test
    agent: "claude --print"
    task: "write tests for the feature"
```

```bash
phantom run-all ~/myproject --config agents.yaml
```

Same behavior as `run-chain` — one overlay, agents run in order.

---

## Safe Experimentation with Snapshots

Save a known-good state before risky changes.

```bash
# Agent has done good work so far
phantom snapshot save feature-a -s "before-refactor"

# Run another agent that might break things
phantom run ~/myproject --agent "claude --print" --task "refactor to microservices" --name feature-a

# It broke things — restore the snapshot
phantom stop feature-a
phantom snapshot restore feature-a feature-a-before-refactor
phantom restart feature-a

# Try a different approach
phantom run ~/myproject --agent "aider" --task "refactor to clean architecture" --name feature-a
```

---

## Combining Parallel Work

Two agents worked on related features. Merge their changes into one overlay before applying.

```bash
# Run agents in parallel
phantom run ~/myproject --agent "claude --print" --task "implement auth" --name auth
phantom run ~/myproject --agent "claude --print" --task "implement user API" --name api

# Wait for both to finish, then check for conflicts
phantom conflicts "auth api"

# If no conflicts, merge api into auth
phantom merge api auth

# Review the combined result
phantom diff auth

# Apply to base
phantom apply auth --cleanup
phantom stop api --cleanup
```

If there are conflicts, use `--force` to overwrite, or manually resolve in the overlay mount point.

---

## Long-Running Protected Overlays

For overlays you want to keep safe from accidental cleanup.

```bash
# Create and lock
phantom start ~/myproject -n experiment-v2
phantom lock experiment-v2

# Work on it over days...
phantom run ~/myproject --agent "claude --print" --task "..." --name experiment-v2

# Pin to the current base commit so you know if the base drifts
phantom pin experiment-v2

# Later, check if base has moved
phantom inspect experiment-v2
# Shows: Pinned: abc123def4

# Sync when ready (will warn about divergence)
phantom sync experiment-v2

# Re-pin after sync
phantom pin experiment-v2

# When done, unlock and cleanup
phantom unlock experiment-v2
phantom apply experiment-v2 --cleanup
```

---

## Post-Run Automation with Hooks

Set up hooks once, they run after every agent execution.

```bash
# Lint on success
phantom hook add --name lint --on success --command "npm run lint --fix 2>/dev/null || true"

# Run tests on success
phantom hook add --name test --on success --command "go test ./... 2>&1 | tail -5"

# Slack notification on failure
phantom hook add --name slack --on failure \
  --command 'curl -s -X POST https://hooks.slack.com/services/XXX -d "{\"text\":\"Agent failed in $OVERLAY_NAME (exit $OVERLAY_EXIT_CODE)\"}"'

# macOS desktop notification
phantom hook add --name desktop --on always \
  --command 'osascript -e "display notification \"$OVERLAY_AGENT finished (exit $OVERLAY_EXIT_CODE)\" with title \"Phantom: $OVERLAY_NAME\""'

# Log all results to CSV
phantom hook add --name log --on always \
  --command 'echo "$(date),$OVERLAY_NAME,$OVERLAY_AGENT,$OVERLAY_EXIT_CODE" >> ~/.phantom/results.csv'
```

View and manage:

```bash
phantom hook list
phantom hook remove slack
```

---

## Quick Retry with Replay

Agent failed or you want to re-run with the same parameters:

```bash
# Check what the last run was
phantom replay feature-a --dry-run
# Output:
#   Would replay in overlay "feature-a":
#     Agent: claude --print
#     Task:  implement authentication module

# Re-run it
phantom replay feature-a
```

---

## Monitoring Agent Progress

Watch what an agent is doing in real time:

```bash
# In terminal 1: run the agent
phantom run ~/myproject --agent "claude --print" --task "implement auth" --name auth

# In terminal 2: watch file changes
phantom watch auth

# In terminal 3: tail the log
phantom logs auth --tail 4096
```

For parallel runs, monitor all agents:

```bash
# Terminal per agent
phantom watch auth-agent
phantom watch api-agent
phantom watch test-agent
```

---

## CI/CD Integration

Use Phantom in CI pipelines for automated agent-driven development.

```bash
#!/bin/bash
set -e

# Run agents
phantom run-all . --config agents.yaml --timeout 30 --cleanup --format json > results.json

# Check results
if jq -e '.[] | select(.exit_code != 0)' results.json > /dev/null 2>&1; then
  echo "Some agents failed"
  exit 1
fi

echo "All agents succeeded"
```

### Using .env Files

Set defaults for CI without passing flags:

```bash
# .env
OVERLAY_AGENT=claude --print
OVERLAY_TASK=implement the feature described in TASK.md
OVERLAY_BRANCH=feature/auto
```

```bash
phantom run .
```

Supported variables: `OVERLAY_CONFIG`, `OVERLAY_NAME`, `OVERLAY_AGENT`, `OVERLAY_TASK`, `OVERLAY_BASE`, `OVERLAY_BRANCH`.

---

## Exporting and Sharing Changes

```bash
# Export as a patch file
phantom export feature-a -o feature-auth.patch

# Apply on another machine
cd ~/myproject
git apply feature-auth.patch

# Or export as a tarball of changed files
phantom export feature-a --format tar -o changes.tar.gz
```

---

## Template-Based Setup

Quickly scaffold agent configs from built-in templates:

```bash
# See available templates
phantom template list

# Generate a config with Claude, Aider, and Gemini
phantom template generate --agents claude,aider,gemini -o agents.yaml

# Edit the tasks
vim agents.yaml

# Run
phantom run-all ~/myproject --config agents.yaml
```

---

## Cloning for Experiments

Branch an overlay to try different approaches:

```bash
# Agent did good work
phantom clone feature-a feature-a-experiment

# Try something risky in the clone
phantom run ~/myproject --agent "claude --print" --task "refactor everything" --name feature-a-experiment

# If it worked, apply the clone. If not, delete it.
phantom stop feature-a-experiment --cleanup
```

---

## Health Check and Maintenance

```bash
# Check system health
phantom health

# Fix issues (remount stale overlays, clean zombies)
phantom health --fix

# Clean up orphaned resources
phantom gc --dry-run
phantom gc

# Remove expired overlays
phantom prune --dry-run
phantom prune
```

---

## Remote Overlay (Multi-Machine Parallel Agents)

Run AI agents on different machines against the same source repo without cloning it.

### Node A — serves the repo

```yaml
# ~/.phantom/config.yaml on Node A
projects:
  myapp:
    path: "/home/user/myapp"
    serve: true
node:
  id: "node-a"
  seeds: []
```

```bash
phantom node start   # run on Node A
```

### Node B — creates a remote overlay

```yaml
# ~/.phantom/config.yaml on Node B
node:
  id: "node-b"
  seeds: ["192.168.1.10:7946"]
```

```bash
phantom node start &
phantom start --repo myapp --node 192.168.1.10 --name agent1
```

### Sync changes back to Node A

```bash
# Explicit push
phantom push agent1 --message "implement feature X"

# Sentinel: agent writes the file, phantom detects and syncs automatically
echo "implement feature X" > ~/.phantom/mnt/agent1/.phantom_commit
# Outcome written to .phantom_commit_result
```

Each node runs `phantom node start` and creates its own overlay with `phantom start --repo myapp --node <addr>`. All overlays share the same read-only base on Node A. Writes are isolated per overlay — push each agent's changes to Node A independently. Files exceeding `node.sync.max_file_size_bytes` are silently skipped during push.

---

## Cross-Network Remote Overlay (Tailscale)

Run agents on machines in different networks without VPN configuration or port forwarding. Both nodes join a [Tailscale](https://tailscale.com) tailnet.

### Node A — serves the repo (e.g. in the cloud)

```yaml
# ~/.phantom/config.yaml on Node A
projects:
  myapp:
    path: "/home/user/myapp"
    serve: true
node:
  tsnet:
    hostname: "phantom-node-a"
    auth_key: "tskey-auth-xxxxx"  # or set TS_AUTHKEY env var
```

```bash
phantom node start   # gRPC server listens on both LAN and Tailscale
```

### Node B — creates a remote overlay (e.g. on a laptop)

```yaml
# ~/.phantom/config.yaml on Node B
node:
  tsnet:
    hostname: "phantom-node-b"
    auth_key: "tskey-auth-xxxxx"
```

```bash
phantom start --repo myapp --node 100.64.1.2:50051 --name agent1
```

The `100.64.1.2` address is Node A's Tailscale IP. Phantom detects it is a Tailscale/CGNAT address and routes the gRPC connection through the mesh automatically. No port forwarding, no TLS certificates — WireGuard encryption is handled by Tailscale.

Sync works the same as LAN remote overlays:

```bash
phantom push agent1 --message "implement feature X"
```
