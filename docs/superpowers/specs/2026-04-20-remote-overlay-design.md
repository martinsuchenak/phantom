# Remote Overlay Design

**Date:** 2026-04-20  
**Status:** Approved

## Overview

Phantom nodes on a local network form a symmetric peer ring. Any node can serve its local repos to other nodes and simultaneously consume remote repos as base layers for overlays. AI agents on a consuming node see a plain local filesystem — they have no knowledge of the remote layer beneath them.

Discovery uses gossip (presence + repo list). File data flows over gRPC/TCP. Writes stay local in the overlay upper dir and are synced back to the source node on demand.

---

## Architecture

```
Node A                                    Node B
──────────────────────────────            ──────────────────────────────────────────
 gossip member                ←──ring──→  gossip member
 gRPC file server (port 50051)            remote FUSE client
 repo: ~/projects/myapp        ←──gRPC──  ~/.phantom/remote-mounts/node-a/myapp/
                                          ↓ (BaseDir)
                                          existing overlay manager (unionfs/fuse-overlayfs)
                                          ↓
                                          ~/.phantom/overlays/agent1/mount/  ← AI agent sees this
                                          sentinel watcher (.phantom_commit)
                                          ↓ (on sync)
                               ──gRPC──→  Node A writes files to working tree
```

Both nodes run identical phantom daemons. Each node simultaneously serves its local repos and can consume remote repos. Roles are per-overlay, not per-node.

---

## Package Structure

Four new internal packages. All existing packages (`internal/overlay/`, `internal/state/`, `internal/git/`, `internal/agent/`) are unchanged.

```
internal/
  node/          # Gossip ring membership + peer/repo registry
    node.go      # Node identity, start/stop gossip member
    registry.go  # In-memory map of peer → repos, updated by gossip events
    peer.go      # Peer type (ID, addr, repo list)

  remotefs/      # FUSE client — mounts a remote repo as a local directory
    mount.go     # Mount/unmount lifecycle
    fs.go        # FUSE filesystem implementation (Lookup, Readdir, Read)
    client.go    # Thin wrapper around the gRPC file client

  rpc/           # gRPC data plane
    proto/       # .proto definitions (FileService)
    server.go    # Serves local repos (Node A side)
    client.go    # Fetches remote files (Node B side)
    auth.go      # Auth middleware (none / secret / mTLS)

  sync/          # Sync engine
    walker.go    # Walks upper dir, collects changed/deleted files
    syncer.go    # Pushes diffs to remote via gRPC
    sentinel.go  # fsnotify watcher for .phantom_commit in overlay root
```

New commands added to `internal/commands/`:
- `node_start.go`, `node_stop.go`, `node_list.go`
- `repos.go`

---

## Gossip Ring (Discovery Plane)

Library: `github.com/paularlott/gossip`

Each node broadcasts:

```go
type NodeMeta struct {
    ID       string   // stable node identity (hostname or UUID from config)
    GRPCAddr string   // "192.168.1.10:50051"
    Repos    []string // ["myapp", "other-project"]
    Version  int      // incremented on repo list change
}
```

Gossip carries presence and repo list only — no file-level metadata.

**Repo resolution for `phantom create --repo myapp`:**
1. Node B searches its local registry for peers advertising `myapp`
2. Exactly one match → connect automatically
3. Multiple matches → error: "ambiguous, use `--node` to specify"
4. No match → error: "repo not found in ring"

Default gossip port: `7946`. Configurable.

---

## gRPC Protocol (Data Plane)

```proto
service FileService {
  rpc ListRepos(ListReposRequest)   returns (ListReposResponse);
  rpc Stat(StatRequest)             returns (StatResponse);
  rpc ReadDir(ReadDirRequest)       returns (ReadDirResponse);
  rpc Read(ReadRequest)             returns (stream ReadChunk);   // server-streaming
  rpc SyncFiles(stream SyncChunk)   returns (SyncResponse);      // client-streaming
}
```

- `Stat` and `ReadDir` are unary — small metadata payloads
- `Read` is server-streaming — Node A streams file bytes in 64KB chunks; large files never buffer fully in memory
- `SyncFiles` is client-streaming — Node B sends one `SyncChunk` per changed file; `deleted: true` signals a deletion

Default gRPC port: `50051`. Configurable.

---

## FUSE Remote Mount Client (`internal/remotefs/`)

Library: `github.com/hanwen/go-fuse/v2`

Node B mounts Node A's repo at `~/.phantom/remote-mounts/<node-id>/<repo>/`. This path becomes `BaseDir` for the existing overlay manager — the overlay stack is unchanged.

| FUSE Operation | Behaviour |
|----------------|-----------|
| `Getattr` / `Lookup` | calls gRPC `Stat()` |
| `Readdir` | calls gRPC `ReadDir()` |
| `Read` | calls gRPC streaming `Read()`, pipes chunks to kernel |
| `Write` / `Create` | returns `EROFS` — base layer is read-only |
| any gRPC error | returns `EIO` immediately — hard fail, no retry, no cache |

**No local caching.** Every read goes to Node A. Stale data is impossible; if Node A is unreachable the agent sees `EIO`.

**Mount lifecycle:**
- Mounted when `phantom create --repo myapp` runs
- Stays mounted for the lifetime of the overlay
- Unmounted after overlay unmount on `phantom destroy`
- If Node A stops while mounted, subsequent reads return `EIO`

---

## Sync Engine (`internal/sync/`)

**Sentinel watcher:**

Uses `github.com/fsnotify/fsnotify` (already a dependency) to watch the overlay mount root. When `.phantom_commit` is written:

1. Read file contents → use as commit message (empty = default message)
2. Trigger sync
3. Delete `.phantom_commit` from the overlay
4. Write `.phantom_commit_result` with outcome (`ok` or error details)

**Sync logic:**

Walk the overlay's raw `UpperDir` (not the mount point):
- Regular files → send via `SyncFiles` stream
- Whiteout files (char device `0:0`) → send as `deleted: true` chunk
- Opaque dirs (xattr `trusted.overlay.opaque=y`) → send as directory replacement

**Node A on receipt:**
1. Write files to working tree (always)
2. Check if path is a git repo (`git rev-parse --git-dir`)
3. If yes → `git add -A && git commit -m "<message>"` (configurable, can be disabled)
4. If no → file writes only
5. Return `SyncResponse` indicating whether a git commit was made

**Explicit sync command:**
```
phantom sync agent1
phantom sync agent1 --message "implement feature X"
```

---

## Auth Tiers

Three modes, configured per-node:

| Mode | Behaviour |
|------|-----------|
| `none` | No credentials. Any node on the network can connect. Default. |
| `secret` | Pre-shared token sent as gRPC metadata and embedded in gossip messages. Also via env `PHANTOM_NODE_SECRET`. |
| `mtls` | Mutual TLS on gRPC connections. Gossip peers verified via the same CA. |

Auth applies to both planes: gossip messages carry the token/CA identity; gRPC connections enforce it at the transport layer. A node ignores gossip messages from peers whose auth doesn't match its configured mode.

Mismatch: Node A returns gRPC `UNAUTHENTICATED`. Phantom surfaces a human-readable error: `"node-a requires authentication, check your node.auth config"`.

---

## Configuration

```yaml
node:
  id: "node-a"              # stable node identity
  gossip_port: 7946
  grpc_port: 50051
  seeds:                    # bootstrap peers for ring entry
    - "192.168.1.10:7946"
  repos:                    # repos this node serves
    - name: "myapp"
      path: "/home/user/myapp"
    - name: "other-project"
      path: "/home/user/other-project"
  auth:
    mode: none              # none | secret | mtls
    secret: ""
    cert_file: ""
    key_file: ""
    ca_file: ""
  sync:
    auto_git_commit: true   # commit on Node A after sync (if git repo)
```

---

## Type Changes (`pkg/api/types.go`)

Additive only:

```go
// Added fields to Overlay
type Overlay struct {
    // ... existing fields unchanged ...
    Remote          bool   // true if base layer is on a remote node
    RemoteNode      string // peer node ID
    RemoteRepo      string // repo name on that node
    RemoteMountPath string // ~/.phantom/remote-mounts/<node>/<repo>
}

// New type
type Peer struct {
    ID       string
    GRPCAddr string
    Repos    []string
}
```

---

## CLI Commands

```
phantom node start                          # start daemon in background
phantom node stop                           # graceful shutdown
phantom node list                           # list peers + their repos

phantom repos                               # all repos visible in ring
                                            # REPO    NODE    ADDRESS
                                            # myapp   node-a  192.168.1.10:50051

phantom create --repo myapp agent1          # auto-discover node
phantom create --repo myapp --node node-a agent1  # explicit node

phantom sync agent1                         # explicit sync
phantom sync agent1 --message "feat: X"
```

---

## New Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/paularlott/gossip` | Gossip ring membership and metadata broadcast |
| `github.com/hanwen/go-fuse/v2` | FUSE filesystem client for remote base layer mount |
| `google.golang.org/grpc` | gRPC server and client |
| `google.golang.org/protobuf` | Proto definitions and serialisation |

`github.com/fsnotify/fsnotify` is already an indirect dependency — promoted to direct use for the sentinel watcher.

---

## What Is Unchanged

- `internal/overlay/` — all platform implementations
- `internal/state/` — state persistence
- `internal/git/` — git operations
- `internal/agent/` — agent runner
- All existing CLI commands
- `pkg/api/types.go` existing fields
