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
