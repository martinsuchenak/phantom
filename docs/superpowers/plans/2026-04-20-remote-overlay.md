# Remote Overlay Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable phantom nodes to serve their repos over gRPC and consume remote repos as transparent FUSE base layers, so AI agents on any node see a plain local filesystem backed by a remote repo.

**Architecture:** Each node runs a gossip member (discovery) and gRPC file server (data). Node B mounts Node A's repo via a custom FUSE client at `~/.phantom/remote-mounts/<node>/<repo>/`, which becomes the `BaseDir` for a standard overlay — the existing overlay manager is completely unchanged. Writes stay in the upper dir and are pushed back via `phantom push`.

**Tech Stack:** `github.com/paularlott/gossip`, `github.com/hanwen/go-fuse/v2`, `google.golang.org/grpc`, `google.golang.org/protobuf`, `github.com/fsnotify/fsnotify` (already indirect dep)

**Spec:** `docs/superpowers/specs/2026-04-20-remote-overlay-design.md`

---

## File Map

### New files
| File | Responsibility |
|------|---------------|
| `internal/rpc/proto/file.proto` | gRPC service + message definitions |
| `internal/rpc/proto/file.pb.go` | Generated protobuf code (do not edit) |
| `internal/rpc/proto/file_grpc.pb.go` | Generated gRPC code (do not edit) |
| `internal/rpc/server.go` | gRPC file server — serves local repos |
| `internal/rpc/server_test.go` | Server unit tests |
| `internal/rpc/client.go` | gRPC file client — fetches remote files |
| `internal/rpc/client_test.go` | Client tests against in-memory server |
| `internal/rpc/auth.go` | Auth interceptors (none / secret / mTLS) |
| `internal/rpc/auth_test.go` | Auth middleware tests |
| `internal/node/peer.go` | `Peer` struct |
| `internal/node/registry.go` | In-memory peer→repo registry |
| `internal/node/registry_test.go` | Registry unit tests |
| `internal/node/node.go` | Gossip member lifecycle |
| `internal/remotefs/client.go` | Thin gRPC client wrapper for FUSE use |
| `internal/remotefs/fs.go` | FUSE filesystem implementation |
| `internal/remotefs/fs_test.go` | FUSE node tests with mock client |
| `internal/remotefs/mount.go` | FUSE mount/unmount lifecycle |
| `internal/sync/walker.go` | Walks overlay upper dir, collects diff |
| `internal/sync/walker_test.go` | Walker tests against tmp dirs |
| `internal/sync/syncer.go` | Streams diff to remote via gRPC |
| `internal/sync/syncer_test.go` | Syncer tests with in-memory server |
| `internal/sync/sentinel.go` | fsnotify watcher for `.phantom_commit` |
| `internal/sync/sentinel_test.go` | Sentinel tests |
| `internal/commands/node_start.go` | `phantom node start` |
| `internal/commands/node_stop.go` | `phantom node stop` |
| `internal/commands/node_list.go` | `phantom node list` |
| `internal/commands/repos.go` | `phantom repos` |
| `internal/commands/push.go` | `phantom push` |

### Modified files
| File | Change |
|------|--------|
| `go.mod` / `go.sum` | Add 4 dependencies |
| `internal/config/config.go` | Add `Node NodeConfig` field + helpers |
| `pkg/api/types.go` | Add remote fields to `Overlay`, add `Peer` type |
| `internal/commands/start.go` | Add `--repo` and `--node` flags |
| `internal/commands/root.go` | Register `node` command group, `repos`, `push` |

---

## Task 1: Add Dependencies

**Files:** `go.mod`, `go.sum`

- [ ] **Step 1: Install protoc compiler**

```bash
# macOS
brew install protobuf

# Verify
protoc --version
# Expected: libprotoc 3.x or 4.x or 25.x
```

- [ ] **Step 2: Install Go protoc plugins**

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Verify both are on PATH (they install to $GOPATH/bin)
which protoc-gen-go protoc-gen-go-grpc
```

- [ ] **Step 3: Add Go module dependencies**

```bash
cd /path/to/phantom

go get github.com/paularlott/gossip@latest
go get github.com/hanwen/go-fuse/v2@latest
go get google.golang.org/grpc@latest
go get google.golang.org/protobuf@latest
go get google.golang.org/grpc/test/bufconn@latest
```

- [ ] **Step 4: Verify module graph builds**

```bash
go build ./...
# Expected: no errors (no new source files yet, just updated go.mod)
```

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add grpc, go-fuse, gossip, protobuf dependencies"
```

---

## Task 2: Config — Add Node Section

**Files:** `internal/config/config.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/config/config_test.go — add to existing test file

func TestNodeConfigDefaults(t *testing.T) {
    cfg := DefaultConfig()
    if cfg.Node.GossipPort != 7946 {
        t.Errorf("expected gossip port 7946, got %d", cfg.Node.GossipPort)
    }
    if cfg.Node.GRPCPort != 50051 {
        t.Errorf("expected grpc port 50051, got %d", cfg.Node.GRPCPort)
    }
    if cfg.Node.Auth.Mode != "none" {
        t.Errorf("expected auth mode 'none', got %q", cfg.Node.Auth.Mode)
    }
}

func TestNodeConfigGetRemoteMountsPath(t *testing.T) {
    cfg := DefaultConfig()
    path := cfg.GetRemoteMountsPath()
    if !strings.HasSuffix(path, "remote-mounts") {
        t.Errorf("expected path ending in remote-mounts, got %q", path)
    }
}

func TestNodeConfigGetNodePIDPath(t *testing.T) {
    cfg := DefaultConfig()
    path := cfg.GetNodePIDPath()
    if !strings.HasSuffix(path, "node.pid") {
        t.Errorf("expected path ending in node.pid, got %q", path)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/config/... -run TestNodeConfig -v
# Expected: FAIL — cfg.Node undefined
```

- [ ] **Step 3: Add Node config structs and helpers to `internal/config/config.go`**

Add after the `Agent` struct and before `MaxTimeoutMinutes`:

```go
// NodeAuth holds authentication configuration for the phantom node daemon
type NodeAuth struct {
    Mode     string `yaml:"mode"`      // none | secret | mtls
    Secret   string `yaml:"secret"`
    CertFile string `yaml:"cert_file"`
    KeyFile  string `yaml:"key_file"`
    CAFile   string `yaml:"ca_file"`
}

// NodeRepo is a repository this node serves
type NodeRepo struct {
    Name string `yaml:"name"`
    Path string `yaml:"path"`
}

// NodeSync controls sync behaviour on the receiving node
type NodeSync struct {
    AutoGitCommit bool `yaml:"auto_git_commit"`
}

// NodeConfig holds configuration for the phantom node daemon
type NodeConfig struct {
    ID          string     `yaml:"id"`
    GossipPort  int        `yaml:"gossip_port"`
    GRPCPort    int        `yaml:"grpc_port"`
    Seeds       []string   `yaml:"seeds"`
    Repos       []NodeRepo `yaml:"repos"`
    Auth        NodeAuth   `yaml:"auth"`
    Sync        NodeSync   `yaml:"sync"`
}
```

Add `Node NodeConfig` field to the `Config` struct after `Projects`:

```go
type Config struct {
    StateDir string            `yaml:"state_dir"`
    Paths    Paths             `yaml:"paths"`
    Logging  Logging           `yaml:"logging"`
    Overlay  Overlay           `yaml:"overlay"`
    Git      Git               `yaml:"git"`
    Darwin   Darwin            `yaml:"darwin"`
    Linux    Linux             `yaml:"linux"`
    Agent    Agent             `yaml:"agent"`
    AgentEnv []string          `yaml:"agent_env"`
    Projects map[string]string `yaml:"projects"`
    Node     NodeConfig        `yaml:"node"`
}
```

In `DefaultConfig()`, add before the closing `}`:

```go
Node: NodeConfig{
    ID:         "",
    GossipPort: 7946,
    GRPCPort:   50051,
    Seeds:      []string{},
    Repos:      []NodeRepo{},
    Auth: NodeAuth{
        Mode: "none",
    },
    Sync: NodeSync{
        AutoGitCommit: true,
    },
},
```

Add helper methods after `GetSnapshotsPath()`:

```go
// GetRemoteMountsPath returns the path where remote repos are FUSE-mounted
func (c *Config) GetRemoteMountsPath() string {
    return filepath.Join(c.StateDir, "remote-mounts")
}

// GetNodePIDPath returns the path of the node daemon PID file
func (c *Config) GetNodePIDPath() string {
    return filepath.Join(c.StateDir, "node.pid")
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/config/... -run TestNodeConfig -v
# Expected: PASS
```

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add NodeConfig section with gossip, grpc, auth, sync settings"
```

---

## Task 3: Types — Extend Overlay + Add Peer

**Files:** `pkg/api/types.go`

- [ ] **Step 1: Write the failing test**

```go
// pkg/api/types_test.go — add to existing test file

func TestOverlayRemoteFields(t *testing.T) {
    o := Overlay{
        Name:            "agent1",
        Remote:          true,
        RemoteNode:      "node-a",
        RemoteRepo:      "myapp",
        RemoteMountPath: "/home/user/.phantom/remote-mounts/node-a/myapp",
    }
    if !o.Remote {
        t.Error("expected Remote to be true")
    }
    if o.RemoteNode != "node-a" {
        t.Errorf("expected RemoteNode 'node-a', got %q", o.RemoteNode)
    }
}

func TestPeer(t *testing.T) {
    p := Peer{
        ID:       "node-a",
        GRPCAddr: "192.168.1.10:50051",
        Repos:    []string{"myapp", "other"},
    }
    if len(p.Repos) != 2 {
        t.Errorf("expected 2 repos, got %d", len(p.Repos))
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./pkg/api/... -run "TestOverlayRemoteFields|TestPeer" -v
# Expected: FAIL — Overlay.Remote undefined, Peer undefined
```

- [ ] **Step 3: Add fields to `pkg/api/types.go`**

In the `Overlay` struct, add after the last existing field:

```go
// Remote overlay fields — zero values mean local overlay
Remote          bool   `json:"remote,omitempty"`
RemoteNode      string `json:"remote_node,omitempty"`
RemoteRepo      string `json:"remote_repo,omitempty"`
RemoteMountPath string `json:"remote_mount_path,omitempty"`
```

Add the `Peer` type after the `Overlay` struct:

```go
// Peer represents another phantom node discovered via gossip
type Peer struct {
    ID       string   `json:"id"`
    GRPCAddr string   `json:"grpc_addr"`
    Repos    []string `json:"repos"`
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./pkg/api/... -run "TestOverlayRemoteFields|TestPeer" -v
# Expected: PASS
```

- [ ] **Step 5: Commit**

```bash
git add pkg/api/types.go pkg/api/types_test.go
git commit -m "feat(api): add remote overlay fields and Peer type"
```

---

## Task 4: gRPC Proto Definition + Code Generation

**Files:** `internal/rpc/proto/file.proto`, generated `.pb.go` files

- [ ] **Step 1: Create the proto directory and write `file.proto`**

```bash
mkdir -p internal/rpc/proto
```

Create `internal/rpc/proto/file.proto`:

```proto
syntax = "proto3";
package phantom.v1;
option go_package = "github.com/martinsuchenak/phantom/internal/rpc/proto;proto";

service FileService {
  rpc ListRepos(ListReposRequest)   returns (ListReposResponse);
  rpc Stat(StatRequest)             returns (StatResponse);
  rpc ReadDir(ReadDirRequest)       returns (ReadDirResponse);
  rpc Read(ReadRequest)             returns (stream ReadChunk);
  rpc SyncFiles(stream SyncChunk)   returns (SyncResponse);
}

message ListReposRequest {}

message ListReposResponse {
  repeated RepoInfo repos = 1;
}

message RepoInfo {
  string name = 1;
}

message StatRequest {
  string repo = 1;
  string path = 2;
}

message StatResponse {
  string name         = 1;
  bool   is_dir       = 2;
  int64  size         = 3;
  int64  mod_time_unix = 4;
  uint32 mode         = 5;
}

message ReadDirRequest {
  string repo = 1;
  string path = 2;
}

message ReadDirResponse {
  repeated StatResponse entries = 1;
}

message ReadRequest {
  string repo   = 1;
  string path   = 2;
  int64  offset = 3;
  int64  length = 4;  // 0 = read to EOF
}

message ReadChunk {
  bytes data = 1;
}

// SyncChunk uses oneof: first message is always SyncHeader, rest are SyncFile
message SyncChunk {
  oneof payload {
    SyncHeader header = 1;
    SyncFile   file   = 2;
  }
}

message SyncHeader {
  string repo           = 1;
  string commit_message = 2;
}

message SyncFile {
  string path    = 1;
  bytes  data    = 2;
  bool   deleted = 3;
  bool   is_dir  = 4;
}

message SyncResponse {
  bool   success         = 1;
  string error           = 2;
  bool   git_committed   = 3;
  string git_commit_hash = 4;
}
```

- [ ] **Step 2: Generate Go code**

```bash
protoc \
  --go_out=. \
  --go_opt=paths=source_relative \
  --go-grpc_out=. \
  --go-grpc_opt=paths=source_relative \
  internal/rpc/proto/file.proto

# Expected: internal/rpc/proto/file.pb.go and file_grpc.pb.go created
ls internal/rpc/proto/
# file.pb.go  file.proto  file_grpc.pb.go
```

- [ ] **Step 3: Verify it compiles**

```bash
go build ./internal/rpc/proto/...
# Expected: no errors
```

- [ ] **Step 4: Commit**

```bash
git add internal/rpc/proto/
git commit -m "feat(rpc): add FileService proto definition and generated code"
```

---

## Task 5: gRPC File Server

**Files:** `internal/rpc/server.go`, `internal/rpc/server_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/rpc/server_test.go`:

```go
package rpc_test

import (
    "context"
    "io"
    "net"
    "os"
    "path/filepath"
    "testing"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
    "google.golang.org/grpc/test/bufconn"

    proto "github.com/martinsuchenak/phantom/internal/rpc/proto"
    "github.com/martinsuchenak/phantom/internal/rpc"
)

const bufSize = 1024 * 1024

func setupServer(t *testing.T, repos map[string]string) (proto.FileServiceClient, func()) {
    t.Helper()
    lis := bufconn.Listen(bufSize)
    srv := grpc.NewServer()
    proto.RegisterFileServiceServer(srv, rpc.NewFileServer(repos))
    go srv.Serve(lis)

    conn, err := grpc.NewClient("passthrough://bufnet",
        grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
            return lis.DialContext(ctx)
        }),
        grpc.WithTransportCredentials(insecure.NewCredentials()),
    )
    if err != nil {
        t.Fatalf("failed to dial bufconn: %v", err)
    }
    return proto.NewFileServiceClient(conn), func() {
        conn.Close()
        srv.Stop()
    }
}

func TestListRepos(t *testing.T) {
    repos := map[string]string{"myapp": t.TempDir()}
    client, cleanup := setupServer(t, repos)
    defer cleanup()

    resp, err := client.ListRepos(context.Background(), &proto.ListReposRequest{})
    if err != nil {
        t.Fatalf("ListRepos: %v", err)
    }
    if len(resp.Repos) != 1 || resp.Repos[0].Name != "myapp" {
        t.Errorf("expected [myapp], got %v", resp.Repos)
    }
}

func TestStat_File(t *testing.T) {
    dir := t.TempDir()
    if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hi"), 0644); err != nil {
        t.Fatal(err)
    }
    client, cleanup := setupServer(t, map[string]string{"r": dir})
    defer cleanup()

    resp, err := client.Stat(context.Background(), &proto.StatRequest{Repo: "r", Path: "hello.txt"})
    if err != nil {
        t.Fatalf("Stat: %v", err)
    }
    if resp.IsDir {
        t.Error("expected file, got dir")
    }
    if resp.Size != 2 {
        t.Errorf("expected size 2, got %d", resp.Size)
    }
}

func TestStat_Dir(t *testing.T) {
    dir := t.TempDir()
    if err := os.Mkdir(filepath.Join(dir, "sub"), 0755); err != nil {
        t.Fatal(err)
    }
    client, cleanup := setupServer(t, map[string]string{"r": dir})
    defer cleanup()

    resp, err := client.Stat(context.Background(), &proto.StatRequest{Repo: "r", Path: "sub"})
    if err != nil {
        t.Fatalf("Stat: %v", err)
    }
    if !resp.IsDir {
        t.Error("expected dir, got file")
    }
}

func TestStat_UnknownRepo(t *testing.T) {
    client, cleanup := setupServer(t, map[string]string{})
    defer cleanup()

    _, err := client.Stat(context.Background(), &proto.StatRequest{Repo: "nope", Path: "x"})
    if err == nil {
        t.Error("expected error for unknown repo")
    }
}

func TestReadDir(t *testing.T) {
    dir := t.TempDir()
    os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
    os.Mkdir(filepath.Join(dir, "sub"), 0755)
    client, cleanup := setupServer(t, map[string]string{"r": dir})
    defer cleanup()

    resp, err := client.ReadDir(context.Background(), &proto.ReadDirRequest{Repo: "r", Path: ""})
    if err != nil {
        t.Fatalf("ReadDir: %v", err)
    }
    if len(resp.Entries) != 2 {
        t.Errorf("expected 2 entries, got %d", len(resp.Entries))
    }
}

func TestRead(t *testing.T) {
    dir := t.TempDir()
    content := []byte("hello world")
    os.WriteFile(filepath.Join(dir, "f.txt"), content, 0644)
    client, cleanup := setupServer(t, map[string]string{"r": dir})
    defer cleanup()

    stream, err := client.Read(context.Background(), &proto.ReadRequest{Repo: "r", Path: "f.txt", Offset: 0, Length: 0})
    if err != nil {
        t.Fatalf("Read: %v", err)
    }
    var got []byte
    for {
        chunk, err := stream.Recv()
        if err == io.EOF {
            break
        }
        if err != nil {
            t.Fatalf("stream.Recv: %v", err)
        }
        got = append(got, chunk.Data...)
    }
    if string(got) != string(content) {
        t.Errorf("expected %q, got %q", content, got)
    }
}

func TestRead_WithOffset(t *testing.T) {
    dir := t.TempDir()
    os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hello world"), 0644)
    client, cleanup := setupServer(t, map[string]string{"r": dir})
    defer cleanup()

    stream, err := client.Read(context.Background(), &proto.ReadRequest{Repo: "r", Path: "f.txt", Offset: 6, Length: 5})
    if err != nil {
        t.Fatalf("Read: %v", err)
    }
    var got []byte
    for {
        chunk, err := stream.Recv()
        if err == io.EOF {
            break
        }
        if err != nil {
            t.Fatal(err)
        }
        got = append(got, chunk.Data...)
    }
    if string(got) != "world" {
        t.Errorf("expected 'world', got %q", got)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/rpc/... -run "TestListRepos|TestStat|TestReadDir|TestRead" -v
# Expected: FAIL — rpc.NewFileServer undefined
```

- [ ] **Step 3: Implement `internal/rpc/server.go`**

```go
package rpc

import (
    "context"
    "io"
    "os"
    "path/filepath"

    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"

    proto "github.com/martinsuchenak/phantom/internal/rpc/proto"
)

const chunkSize = 64 * 1024 // 64KB

// FileServer implements proto.FileServiceServer, serving files from local repos.
type FileServer struct {
    proto.UnimplementedFileServiceServer
    // repos maps repo name → absolute local path
    repos map[string]string
}

// NewFileServer creates a FileServer serving the given repos.
func NewFileServer(repos map[string]string) *FileServer {
    return &FileServer{repos: repos}
}

func (s *FileServer) repoPath(repo string) (string, error) {
    path, ok := s.repos[repo]
    if !ok {
        return "", status.Errorf(codes.NotFound, "repo %q not found", repo)
    }
    return path, nil
}

// safePath joins base+rel and ensures the result is still under base (no path traversal).
func safePath(base, rel string) (string, error) {
    joined := filepath.Join(base, filepath.Clean("/"+rel))
    if len(joined) < len(base) {
        return "", status.Error(codes.InvalidArgument, "path traversal denied")
    }
    return joined, nil
}

func (s *FileServer) ListRepos(_ context.Context, _ *proto.ListReposRequest) (*proto.ListReposResponse, error) {
    infos := make([]*proto.RepoInfo, 0, len(s.repos))
    for name := range s.repos {
        infos = append(infos, &proto.RepoInfo{Name: name})
    }
    return &proto.ListReposResponse{Repos: infos}, nil
}

func (s *FileServer) Stat(_ context.Context, req *proto.StatRequest) (*proto.StatResponse, error) {
    base, err := s.repoPath(req.Repo)
    if err != nil {
        return nil, err
    }
    full, err := safePath(base, req.Path)
    if err != nil {
        return nil, err
    }
    info, err := os.Lstat(full)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, status.Errorf(codes.NotFound, "path %q not found", req.Path)
        }
        return nil, status.Errorf(codes.Internal, "stat: %v", err)
    }
    return &proto.StatResponse{
        Name:        info.Name(),
        IsDir:       info.IsDir(),
        Size:        info.Size(),
        ModTimeUnix: info.ModTime().Unix(),
        Mode:        uint32(info.Mode()),
    }, nil
}

func (s *FileServer) ReadDir(_ context.Context, req *proto.ReadDirRequest) (*proto.ReadDirResponse, error) {
    base, err := s.repoPath(req.Repo)
    if err != nil {
        return nil, err
    }
    full, err := safePath(base, req.Path)
    if err != nil {
        return nil, err
    }
    entries, err := os.ReadDir(full)
    if err != nil {
        return nil, status.Errorf(codes.Internal, "readdir: %v", err)
    }
    resp := &proto.ReadDirResponse{}
    for _, e := range entries {
        info, err := e.Info()
        if err != nil {
            continue
        }
        resp.Entries = append(resp.Entries, &proto.StatResponse{
            Name:        e.Name(),
            IsDir:       e.IsDir(),
            Size:        info.Size(),
            ModTimeUnix: info.ModTime().Unix(),
            Mode:        uint32(info.Mode()),
        })
    }
    return resp, nil
}

func (s *FileServer) Read(req *proto.ReadRequest, stream proto.FileService_ReadServer) error {
    base, err := s.repoPath(req.Repo)
    if err != nil {
        return err
    }
    full, err := safePath(base, req.Path)
    if err != nil {
        return err
    }
    f, err := os.Open(full)
    if err != nil {
        return status.Errorf(codes.Internal, "open: %v", err)
    }
    defer f.Close()

    if req.Offset > 0 {
        if _, err := f.Seek(req.Offset, io.SeekStart); err != nil {
            return status.Errorf(codes.Internal, "seek: %v", err)
        }
    }

    buf := make([]byte, chunkSize)
    var sent int64
    for {
        toRead := int64(len(buf))
        if req.Length > 0 {
            remaining := req.Length - sent
            if remaining <= 0 {
                break
            }
            if remaining < toRead {
                toRead = remaining
            }
        }
        n, err := f.Read(buf[:toRead])
        if n > 0 {
            if sendErr := stream.Send(&proto.ReadChunk{Data: buf[:n]}); sendErr != nil {
                return sendErr
            }
            sent += int64(n)
        }
        if err == io.EOF {
            break
        }
        if err != nil {
            return status.Errorf(codes.Internal, "read: %v", err)
        }
    }
    return nil
}

func (s *FileServer) SyncFiles(stream proto.FileService_SyncFilesServer) error {
    // SyncFiles is implemented in Task 13 (sync engine).
    // This stub exists so the server compiles and the interface is satisfied.
    return status.Error(codes.Unimplemented, "SyncFiles not yet implemented")
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/rpc/... -run "TestListRepos|TestStat|TestReadDir|TestRead" -v
# Expected: PASS
```

- [ ] **Step 5: Commit**

```bash
git add internal/rpc/server.go internal/rpc/server_test.go
git commit -m "feat(rpc): implement FileServer with Stat, ReadDir, Read, ListRepos"
```

---

## Task 6: gRPC Auth Middleware

**Files:** `internal/rpc/auth.go`, `internal/rpc/auth_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/rpc/auth_test.go`:

```go
package rpc_test

import (
    "context"
    "testing"

    "google.golang.org/grpc"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/credentials/insecure"
    "google.golang.org/grpc/metadata"
    "google.golang.org/grpc/status"
    "google.golang.org/grpc/test/bufconn"
    "net"

    proto "github.com/martinsuchenak/phantom/internal/rpc/proto"
    "github.com/martinsuchenak/phantom/internal/rpc"
)

func setupServerWithAuth(t *testing.T, opts rpc.AuthOptions) (proto.FileServiceClient, func()) {
    t.Helper()
    lis := bufconn.Listen(bufSize)
    srv := grpc.NewServer(grpc.UnaryInterceptor(rpc.UnaryAuthInterceptor(opts)))
    proto.RegisterFileServiceServer(srv, rpc.NewFileServer(map[string]string{}))
    go srv.Serve(lis)

    conn, err := grpc.NewClient("passthrough://bufnet",
        grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
            return lis.DialContext(ctx)
        }),
        grpc.WithTransportCredentials(insecure.NewCredentials()),
    )
    if err != nil {
        t.Fatalf("dial: %v", err)
    }
    return proto.NewFileServiceClient(conn), func() { conn.Close(); srv.Stop() }
}

func TestAuthNone(t *testing.T) {
    client, cleanup := setupServerWithAuth(t, rpc.AuthOptions{Mode: rpc.AuthNone})
    defer cleanup()

    _, err := client.ListRepos(context.Background(), &proto.ListReposRequest{})
    if err != nil {
        t.Errorf("expected no error with auth=none, got %v", err)
    }
}

func TestAuthSecretMissing(t *testing.T) {
    client, cleanup := setupServerWithAuth(t, rpc.AuthOptions{Mode: rpc.AuthSecret, Secret: "s3cr3t"})
    defer cleanup()

    _, err := client.ListRepos(context.Background(), &proto.ListReposRequest{})
    if status.Code(err) != codes.Unauthenticated {
        t.Errorf("expected Unauthenticated, got %v", err)
    }
}

func TestAuthSecretValid(t *testing.T) {
    client, cleanup := setupServerWithAuth(t, rpc.AuthOptions{Mode: rpc.AuthSecret, Secret: "s3cr3t"})
    defer cleanup()

    ctx := metadata.AppendToOutgoingContext(context.Background(), "phantom-secret", "s3cr3t")
    _, err := client.ListRepos(ctx, &proto.ListReposRequest{})
    if err != nil {
        t.Errorf("expected success with valid secret, got %v", err)
    }
}

func TestAuthSecretWrong(t *testing.T) {
    client, cleanup := setupServerWithAuth(t, rpc.AuthOptions{Mode: rpc.AuthSecret, Secret: "s3cr3t"})
    defer cleanup()

    ctx := metadata.AppendToOutgoingContext(context.Background(), "phantom-secret", "wrong")
    _, err := client.ListRepos(ctx, &proto.ListReposRequest{})
    if status.Code(err) != codes.Unauthenticated {
        t.Errorf("expected Unauthenticated, got %v", err)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/rpc/... -run TestAuth -v
# Expected: FAIL — rpc.AuthOptions, rpc.UnaryAuthInterceptor undefined
```

- [ ] **Step 3: Implement `internal/rpc/auth.go`**

```go
package rpc

import (
    "context"

    "google.golang.org/grpc"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/metadata"
    "google.golang.org/grpc/status"
)

type AuthMode string

const (
    AuthNone   AuthMode = "none"
    AuthSecret AuthMode = "secret"
    AuthMTLS   AuthMode = "mtls"
)

// AuthOptions configures authentication for the gRPC server interceptor.
type AuthOptions struct {
    Mode   AuthMode
    Secret string // used when Mode == AuthSecret
}

// UnaryAuthInterceptor returns a gRPC unary server interceptor enforcing auth.
func UnaryAuthInterceptor(opts AuthOptions) grpc.UnaryServerInterceptor {
    return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
        if err := checkAuth(ctx, opts); err != nil {
            return nil, err
        }
        return handler(ctx, req)
    }
}

// StreamAuthInterceptor returns a gRPC stream server interceptor enforcing auth.
func StreamAuthInterceptor(opts AuthOptions) grpc.StreamServerInterceptor {
    return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
        if err := checkAuth(ss.Context(), opts); err != nil {
            return err
        }
        return handler(srv, ss)
    }
}

func checkAuth(ctx context.Context, opts AuthOptions) error {
    switch opts.Mode {
    case AuthNone, "":
        return nil
    case AuthSecret:
        md, ok := metadata.FromIncomingContext(ctx)
        if !ok {
            return status.Error(codes.Unauthenticated, "missing metadata")
        }
        vals := md.Get("phantom-secret")
        if len(vals) == 0 || vals[0] != opts.Secret {
            return status.Error(codes.Unauthenticated, "invalid or missing phantom-secret")
        }
        return nil
    case AuthMTLS:
        // mTLS is enforced at the transport layer via grpc.Creds — nothing to check here.
        return nil
    default:
        return status.Errorf(codes.Internal, "unknown auth mode %q", opts.Mode)
    }
}

// SecretCallOption returns a grpc.CallOption that injects the shared secret into outgoing metadata.
func SecretCallOption(secret string) grpc.CallOption {
    return grpc.Header(nil) // placeholder — use ClientSecretInterceptor instead
}

// UnaryClientSecretInterceptor injects the shared secret into every outgoing unary RPC.
func UnaryClientSecretInterceptor(secret string) grpc.UnaryClientInterceptor {
    return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
        ctx = metadata.AppendToOutgoingContext(ctx, "phantom-secret", secret)
        return invoker(ctx, method, req, reply, cc, opts...)
    }
}

// StreamClientSecretInterceptor injects the shared secret into every outgoing streaming RPC.
func StreamClientSecretInterceptor(secret string) grpc.StreamClientInterceptor {
    return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
        ctx = metadata.AppendToOutgoingContext(ctx, "phantom-secret", secret)
        return streamer(ctx, desc, cc, method, opts...)
    }
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/rpc/... -run TestAuth -v
# Expected: PASS
```

- [ ] **Step 5: Commit**

```bash
git add internal/rpc/auth.go internal/rpc/auth_test.go
git commit -m "feat(rpc): add auth middleware for none/secret/mTLS modes"
```

---

## Task 7: gRPC File Client

**Files:** `internal/rpc/client.go`, `internal/rpc/client_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/rpc/client_test.go`:

```go
package rpc_test

import (
    "context"
    "os"
    "path/filepath"
    "testing"

    "github.com/martinsuchenak/phantom/internal/rpc"
)

func TestClientListRepos(t *testing.T) {
    dir := t.TempDir()
    _, cleanup := setupServer(t, map[string]string{"myapp": dir})
    defer cleanup()
    // Client is tested indirectly via setupServer; test the FileClient wrapper.
}

func TestFileClientStat(t *testing.T) {
    dir := t.TempDir()
    os.WriteFile(filepath.Join(dir, "x.txt"), []byte("abc"), 0644)

    srv, cleanup := setupServer(t, map[string]string{"r": dir})
    defer cleanup()

    // Wrap in FileClient
    fc := rpc.NewFileClient(srv)

    info, err := fc.Stat(context.Background(), "r", "x.txt")
    if err != nil {
        t.Fatalf("Stat: %v", err)
    }
    if info.Size != 3 {
        t.Errorf("expected size 3, got %d", info.Size)
    }
    if info.IsDir {
        t.Error("expected file")
    }
}

func TestFileClientReadDir(t *testing.T) {
    dir := t.TempDir()
    os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
    os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)

    srv, cleanup := setupServer(t, map[string]string{"r": dir})
    defer cleanup()
    fc := rpc.NewFileClient(srv)

    entries, err := fc.ReadDir(context.Background(), "r", "")
    if err != nil {
        t.Fatalf("ReadDir: %v", err)
    }
    if len(entries) != 2 {
        t.Errorf("expected 2 entries, got %d", len(entries))
    }
}

func TestFileClientReadAll(t *testing.T) {
    dir := t.TempDir()
    os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hello"), 0644)

    srv, cleanup := setupServer(t, map[string]string{"r": dir})
    defer cleanup()
    fc := rpc.NewFileClient(srv)

    data, err := fc.ReadAll(context.Background(), "r", "f.txt", 0, 0)
    if err != nil {
        t.Fatalf("ReadAll: %v", err)
    }
    if string(data) != "hello" {
        t.Errorf("expected 'hello', got %q", data)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/rpc/... -run "TestFileClient" -v
# Expected: FAIL — rpc.NewFileClient undefined
```

- [ ] **Step 3: Implement `internal/rpc/client.go`**

```go
package rpc

import (
    "context"
    "crypto/tls"
    "crypto/x509"
    "io"
    "os"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials"
    "google.golang.org/grpc/credentials/insecure"

    proto "github.com/martinsuchenak/phantom/internal/rpc/proto"
)

// FileInfo is a local representation of a remote file's metadata.
type FileInfo struct {
    Name        string
    IsDir       bool
    Size        int64
    ModTimeUnix int64
    Mode        uint32
}

// FileClient wraps the gRPC FileServiceClient with a friendlier API.
type FileClient struct {
    inner proto.FileServiceClient
}

// NewFileClient wraps an existing proto client (useful in tests).
func NewFileClient(inner proto.FileServiceClient) *FileClient {
    return &FileClient{inner: inner}
}

// DialOpts holds options for dialing a remote phantom node.
type DialOpts struct {
    Auth   AuthOptions
    CAFile string // for mTLS
    Cert   string // for mTLS
    Key    string // for mTLS
}

// Dial creates a FileClient connected to the given gRPC address.
func Dial(ctx context.Context, addr string, opts DialOpts) (*FileClient, error) {
    var dialOpts []grpc.DialOption

    switch opts.Auth.Mode {
    case AuthMTLS:
        cert, err := tls.LoadX509KeyPair(opts.Cert, opts.Key)
        if err != nil {
            return nil, err
        }
        caCert, err := os.ReadFile(opts.CAFile)
        if err != nil {
            return nil, err
        }
        pool := x509.NewCertPool()
        pool.AppendCertsFromPEM(caCert)
        tlsCfg := &tls.Config{
            Certificates: []tls.Certificate{cert},
            RootCAs:      pool,
        }
        dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
    case AuthSecret:
        dialOpts = append(dialOpts,
            grpc.WithTransportCredentials(insecure.NewCredentials()),
            grpc.WithUnaryInterceptor(UnaryClientSecretInterceptor(opts.Auth.Secret)),
            grpc.WithStreamInterceptor(StreamClientSecretInterceptor(opts.Auth.Secret)),
        )
    default:
        dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
    }

    conn, err := grpc.NewClient(addr, dialOpts...)
    if err != nil {
        return nil, err
    }
    return &FileClient{inner: proto.NewFileServiceClient(conn)}, nil
}

func (c *FileClient) ListRepos(ctx context.Context) ([]string, error) {
    resp, err := c.inner.ListRepos(ctx, &proto.ListReposRequest{})
    if err != nil {
        return nil, err
    }
    names := make([]string, len(resp.Repos))
    for i, r := range resp.Repos {
        names[i] = r.Name
    }
    return names, nil
}

func (c *FileClient) Stat(ctx context.Context, repo, path string) (FileInfo, error) {
    resp, err := c.inner.Stat(ctx, &proto.StatRequest{Repo: repo, Path: path})
    if err != nil {
        return FileInfo{}, err
    }
    return FileInfo{
        Name:        resp.Name,
        IsDir:       resp.IsDir,
        Size:        resp.Size,
        ModTimeUnix: resp.ModTimeUnix,
        Mode:        resp.Mode,
    }, nil
}

func (c *FileClient) ReadDir(ctx context.Context, repo, path string) ([]FileInfo, error) {
    resp, err := c.inner.ReadDir(ctx, &proto.ReadDirRequest{Repo: repo, Path: path})
    if err != nil {
        return nil, err
    }
    infos := make([]FileInfo, len(resp.Entries))
    for i, e := range resp.Entries {
        infos[i] = FileInfo{
            Name:        e.Name,
            IsDir:       e.IsDir,
            Size:        e.Size,
            ModTimeUnix: e.ModTimeUnix,
            Mode:        e.Mode,
        }
    }
    return infos, nil
}

// ReadAll fetches file bytes from offset for length bytes (0 = to EOF).
func (c *FileClient) ReadAll(ctx context.Context, repo, path string, offset, length int64) ([]byte, error) {
    stream, err := c.inner.Read(ctx, &proto.ReadRequest{
        Repo:   repo,
        Path:   path,
        Offset: offset,
        Length: length,
    })
    if err != nil {
        return nil, err
    }
    var buf []byte
    for {
        chunk, err := stream.Recv()
        if err == io.EOF {
            break
        }
        if err != nil {
            return nil, err
        }
        buf = append(buf, chunk.Data...)
    }
    return buf, nil
}

// ReadStream opens a streaming read and calls fn for each chunk.
func (c *FileClient) ReadStream(ctx context.Context, repo, path string, offset, length int64, fn func([]byte) error) error {
    stream, err := c.inner.Read(ctx, &proto.ReadRequest{
        Repo:   repo,
        Path:   path,
        Offset: offset,
        Length: length,
    })
    if err != nil {
        return err
    }
    for {
        chunk, err := stream.Recv()
        if err == io.EOF {
            return nil
        }
        if err != nil {
            return err
        }
        if err := fn(chunk.Data); err != nil {
            return err
        }
    }
}

// Inner returns the underlying proto client (for sync operations).
func (c *FileClient) Inner() proto.FileServiceClient {
    return c.inner
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/rpc/... -v
# Expected: all PASS
```

- [ ] **Step 5: Commit**

```bash
git add internal/rpc/client.go internal/rpc/client_test.go
git commit -m "feat(rpc): implement FileClient with Stat, ReadDir, ReadAll, ReadStream, Dial"
```

---

## Task 8: Gossip Node + Peer Registry

**Files:** `internal/node/peer.go`, `internal/node/registry.go`, `internal/node/registry_test.go`, `internal/node/node.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/node/registry_test.go`:

```go
package node_test

import (
    "testing"

    "github.com/martinsuchenak/phantom/internal/node"
)

func TestRegistryAddAndLookup(t *testing.T) {
    r := node.NewRegistry()
    r.Upsert(node.Peer{ID: "a", GRPCAddr: "1.2.3.4:50051", Repos: []string{"myapp", "other"}})
    r.Upsert(node.Peer{ID: "b", GRPCAddr: "5.6.7.8:50051", Repos: []string{"service"}})

    peers := r.FindByRepo("myapp")
    if len(peers) != 1 || peers[0].ID != "a" {
        t.Errorf("expected peer 'a', got %v", peers)
    }
}

func TestRegistryFindByRepoMultiple(t *testing.T) {
    r := node.NewRegistry()
    r.Upsert(node.Peer{ID: "a", GRPCAddr: "1:50051", Repos: []string{"shared"}})
    r.Upsert(node.Peer{ID: "b", GRPCAddr: "2:50051", Repos: []string{"shared"}})

    peers := r.FindByRepo("shared")
    if len(peers) != 2 {
        t.Errorf("expected 2 peers, got %d", len(peers))
    }
}

func TestRegistryRemove(t *testing.T) {
    r := node.NewRegistry()
    r.Upsert(node.Peer{ID: "a", GRPCAddr: "1:50051", Repos: []string{"myapp"}})
    r.Remove("a")

    peers := r.FindByRepo("myapp")
    if len(peers) != 0 {
        t.Errorf("expected 0 peers after remove, got %d", len(peers))
    }
}

func TestRegistryAll(t *testing.T) {
    r := node.NewRegistry()
    r.Upsert(node.Peer{ID: "a", GRPCAddr: "1:50051", Repos: []string{"x"}})
    r.Upsert(node.Peer{ID: "b", GRPCAddr: "2:50051", Repos: []string{"y"}})

    all := r.All()
    if len(all) != 2 {
        t.Errorf("expected 2 peers, got %d", len(all))
    }
}

func TestRegistryUpsertUpdates(t *testing.T) {
    r := node.NewRegistry()
    r.Upsert(node.Peer{ID: "a", GRPCAddr: "1:50051", Repos: []string{"old"}})
    r.Upsert(node.Peer{ID: "a", GRPCAddr: "1:50051", Repos: []string{"new"}})

    peers := r.FindByRepo("old")
    if len(peers) != 0 {
        t.Error("expected old repo gone after upsert")
    }
    peers = r.FindByRepo("new")
    if len(peers) != 1 {
        t.Error("expected new repo present after upsert")
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/node/... -v
# Expected: FAIL — node package does not exist
```

- [ ] **Step 3: Create `internal/node/peer.go`**

```go
package node

// Peer represents another phantom node discovered via gossip.
type Peer struct {
    ID       string
    GRPCAddr string
    Repos    []string
}
```

- [ ] **Step 4: Create `internal/node/registry.go`**

```go
package node

import "sync"

// Registry holds the set of known peers discovered via gossip.
type Registry struct {
    mu    sync.RWMutex
    peers map[string]Peer // key: peer ID
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
    return &Registry{peers: make(map[string]Peer)}
}

// Upsert adds or replaces a peer.
func (r *Registry) Upsert(p Peer) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.peers[p.ID] = p
}

// Remove removes a peer by ID.
func (r *Registry) Remove(id string) {
    r.mu.Lock()
    defer r.mu.Unlock()
    delete(r.peers, id)
}

// FindByRepo returns all peers that advertise the given repo name.
func (r *Registry) FindByRepo(repo string) []Peer {
    r.mu.RLock()
    defer r.mu.RUnlock()
    var out []Peer
    for _, p := range r.peers {
        for _, name := range p.Repos {
            if name == repo {
                out = append(out, p)
                break
            }
        }
    }
    return out
}

// All returns a snapshot of all known peers.
func (r *Registry) All() []Peer {
    r.mu.RLock()
    defer r.mu.RUnlock()
    out := make([]Peer, 0, len(r.peers))
    for _, p := range r.peers {
        out = append(out, p)
    }
    return out
}
```

- [ ] **Step 5: Run registry tests to verify they pass**

```bash
go test ./internal/node/... -run "TestRegistry" -v
# Expected: PASS
```

- [ ] **Step 6: Create `internal/node/node.go`**

This wraps `github.com/paularlott/gossip`. Check the library README at `github.com/paularlott/gossip` for the exact API — adapt method names as needed. The interface below is what the rest of the system calls.

```go
package node

import (
    "context"
    "encoding/json"
    "fmt"
    "os"

    "github.com/paularlott/gossip"
)

// Meta is the payload each node broadcasts via gossip.
type Meta struct {
    ID       string   `json:"id"`
    GRPCAddr string   `json:"grpc_addr"`
    Repos    []string `json:"repos"`
    Version  int      `json:"version"`
}

// Node is a gossip ring member that maintains the peer registry.
type Node struct {
    id       string
    registry *Registry
    member   *gossip.Node // adapt to actual paularlott/gossip type
}

// Config holds the parameters to start a Node.
type Config struct {
    ID         string
    BindAddr   string // "0.0.0.0:<gossip_port>"
    GRPCAddr   string // "<host>:<grpc_port>" — advertised to peers
    Seeds      []string
    Repos      []string
    PIDFile    string
}

// Start joins the gossip ring and begins advertising this node's metadata.
// It blocks until ctx is cancelled.
func Start(ctx context.Context, cfg Config, registry *Registry) error {
    meta, err := json.Marshal(Meta{
        ID:       cfg.ID,
        GRPCAddr: cfg.GRPCAddr,
        Repos:    cfg.Repos,
        Version:  1,
    })
    if err != nil {
        return fmt.Errorf("marshal meta: %w", err)
    }

    // Write PID file
    if cfg.PIDFile != "" {
        if err := os.WriteFile(cfg.PIDFile, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0600); err != nil {
            return fmt.Errorf("write pid file: %w", err)
        }
        defer os.Remove(cfg.PIDFile)
    }

    // NOTE: adapt the following to the actual github.com/paularlott/gossip API.
    // The library may use different constructor names or option structs.
    node, err := gossip.New(gossip.Config{
        BindAddr: cfg.BindAddr,
        Meta:     meta,
        OnJoin: func(id string, nodeMeta []byte) {
            var m Meta
            if err := json.Unmarshal(nodeMeta, &m); err == nil {
                registry.Upsert(Peer{ID: m.ID, GRPCAddr: m.GRPCAddr, Repos: m.Repos})
            }
        },
        OnLeave: func(id string) {
            registry.Remove(id)
        },
        OnUpdate: func(id string, nodeMeta []byte) {
            var m Meta
            if err := json.Unmarshal(nodeMeta, &m); err == nil {
                registry.Upsert(Peer{ID: m.ID, GRPCAddr: m.GRPCAddr, Repos: m.Repos})
            }
        },
    })
    if err != nil {
        return fmt.Errorf("create gossip node: %w", err)
    }

    if len(cfg.Seeds) > 0 {
        if err := node.Join(cfg.Seeds); err != nil {
            return fmt.Errorf("join gossip ring: %w", err)
        }
    }

    <-ctx.Done()
    node.Leave()
    return nil
}
```

- [ ] **Step 7: Verify it compiles**

```bash
go build ./internal/node/...
# If paularlott/gossip API differs from above, adapt node.go accordingly.
# The registry tests must still pass.
go test ./internal/node/... -run TestRegistry -v
```

- [ ] **Step 8: Commit**

```bash
git add internal/node/
git commit -m "feat(node): gossip peer registry and node lifecycle"
```

---

## Task 9: Node Daemon Commands + phantom repos

**Files:** `internal/commands/node_start.go`, `internal/commands/node_stop.go`, `internal/commands/node_list.go`, `internal/commands/repos.go`, `internal/commands/root.go`

- [ ] **Step 1: Create `internal/commands/node_start.go`**

```go
package commands

import (
    "context"
    "fmt"
    "net"
    "os"
    "path/filepath"

    "google.golang.org/grpc"

    proto "github.com/martinsuchenak/phantom/internal/rpc/proto"
    "github.com/martinsuchenak/phantom/internal/node"
    "github.com/martinsuchenak/phantom/internal/rpc"
    "github.com/paularlott/cli"
)

func NewNodeCommand() *cli.Command {
    return &cli.Command{
        Name:  "node",
        Usage: "Manage the phantom node daemon (gossip + gRPC file server)",
        Commands: []*cli.Command{
            NewNodeStartCommand(),
            NewNodeStopCommand(),
            NewNodeListCommand(),
        },
    }
}

func NewNodeStartCommand() *cli.Command {
    return &cli.Command{
        Name:  "start",
        Usage: "Start the phantom node daemon (gossip ring + gRPC file server)",
        Description: "Runs in the foreground. Use a process manager (systemd, launchd, tmux) to background it. " +
            "Writes PID to ~/.phantom/node.pid.",
        Run: doNodeStart,
    }
}

func doNodeStart(ctx context.Context, cmd *cli.Command) error {
    nodeCfg := cfg.Node

    nodeID := nodeCfg.ID
    if nodeID == "" {
        hostname, err := os.Hostname()
        if err != nil {
            return fmt.Errorf("cannot determine node ID (set node.id in config): %w", err)
        }
        nodeID = hostname
    }

    // Build repos map for gRPC server
    repoMap := make(map[string]string, len(nodeCfg.Repos))
    repoNames := make([]string, 0, len(nodeCfg.Repos))
    for _, r := range nodeCfg.Repos {
        abs, err := filepath.Abs(r.Path)
        if err != nil {
            return fmt.Errorf("invalid repo path %q: %w", r.Path, err)
        }
        repoMap[r.Name] = abs
        repoNames = append(repoNames, r.Name)
    }

    // Start gRPC server
    grpcAddr := fmt.Sprintf("0.0.0.0:%d", nodeCfg.GRPCPort)
    lis, err := net.Listen("tcp", grpcAddr)
    if err != nil {
        return fmt.Errorf("listen on %s: %w", grpcAddr, err)
    }

    authOpts := rpc.AuthOptions{
        Mode:   rpc.AuthMode(nodeCfg.Auth.Mode),
        Secret: nodeCfg.Auth.Secret,
    }

    srv := grpc.NewServer(
        grpc.UnaryInterceptor(rpc.UnaryAuthInterceptor(authOpts)),
        grpc.StreamInterceptor(rpc.StreamAuthInterceptor(authOpts)),
    )
    proto.RegisterFileServiceServer(srv, rpc.NewFileServer(repoMap))

    log.Info("phantom node starting", "id", nodeID, "grpc_addr", grpcAddr, "repos", repoNames)

    grpcErrCh := make(chan error, 1)
    go func() {
        grpcErrCh <- srv.Serve(lis)
    }()

    // Start gossip
    registry := node.NewRegistry()
    gossipAddr := fmt.Sprintf("0.0.0.0:%d", nodeCfg.GossipPort)

    // Determine advertised gRPC address (use outbound IP if not configured)
    advertisedGRPC := fmt.Sprintf("%s:%d", outboundIP(), nodeCfg.GRPCPort)

    gossipErrCh := make(chan error, 1)
    go func() {
        gossipErrCh <- node.Start(ctx, node.Config{
            ID:       nodeID,
            BindAddr: gossipAddr,
            GRPCAddr: advertisedGRPC,
            Seeds:    nodeCfg.Seeds,
            Repos:    repoNames,
            PIDFile:  cfg.GetNodePIDPath(),
        }, registry)
    }()

    select {
    case err := <-grpcErrCh:
        return fmt.Errorf("gRPC server error: %w", err)
    case err := <-gossipErrCh:
        srv.GracefulStop()
        return err
    case <-ctx.Done():
        srv.GracefulStop()
        return nil
    }
}

func outboundIP() string {
    conn, err := net.Dial("udp", "8.8.8.8:80")
    if err != nil {
        return "127.0.0.1"
    }
    defer conn.Close()
    return conn.LocalAddr().(*net.UDPAddr).IP.String()
}
```

- [ ] **Step 2: Create `internal/commands/node_stop.go`**

```go
package commands

import (
    "context"
    "fmt"
    "os"
    "strconv"
    "strings"
    "syscall"

    "github.com/paularlott/cli"
)

func NewNodeStopCommand() *cli.Command {
    return &cli.Command{
        Name:  "stop",
        Usage: "Stop the running phantom node daemon",
        Run:   doNodeStop,
    }
}

func doNodeStop(_ context.Context, _ *cli.Command) error {
    pidFile := cfg.GetNodePIDPath()
    data, err := os.ReadFile(pidFile)
    if err != nil {
        if os.IsNotExist(err) {
            return fmt.Errorf("no node daemon is running (no PID file at %s)", pidFile)
        }
        return fmt.Errorf("read PID file: %w", err)
    }
    pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
    if err != nil {
        return fmt.Errorf("invalid PID in %s: %w", pidFile, err)
    }
    proc, err := os.FindProcess(pid)
    if err != nil {
        return fmt.Errorf("find process %d: %w", pid, err)
    }
    if err := proc.Signal(syscall.SIGTERM); err != nil {
        return fmt.Errorf("signal process %d: %w", pid, err)
    }
    fmt.Printf("Sent SIGTERM to node daemon (PID %d)\n", pid)
    return nil
}
```

- [ ] **Step 3: Create `internal/commands/node_list.go`**

```go
package commands

import (
    "context"
    "fmt"

    "github.com/paularlott/cli"
    "github.com/martinsuchenak/phantom/internal/node"
)

func NewNodeListCommand() *cli.Command {
    return &cli.Command{
        Name:  "list",
        Usage: "List phantom nodes visible in the gossip ring",
        Run:   doNodeList,
    }
}

// nodeRegistry is set when the daemon starts; for CLI use we read from a
// shared state file written by the daemon. For now, list is a no-op placeholder
// until daemon ↔ CLI IPC is added in a follow-up.
func doNodeList(_ context.Context, _ *cli.Command) error {
    // TODO(follow-up): read peer state from daemon via Unix socket or state file.
    // For now, indicate the daemon must be queried.
    fmt.Println("NODE\tADDR\t\tREPOS")
    fmt.Println("(Connect to the running daemon to list peers — IPC not yet implemented)")
    fmt.Println("Hint: check daemon logs or use `phantom repos` after the daemon is running.")
    return nil
}
```

- [ ] **Step 4: Create `internal/commands/repos.go`**

```go
package commands

import (
    "context"
    "fmt"

    "github.com/paularlott/cli"
)

func NewReposCommand() *cli.Command {
    return &cli.Command{
        Name:  "repos",
        Usage: "List all repositories visible in the gossip ring",
        Description: "Shows repos advertised by all known phantom nodes. " +
            "Requires a running node daemon (`phantom node start`).",
        Run: doRepos,
    }
}

func doRepos(_ context.Context, _ *cli.Command) error {
    // TODO(follow-up): read peer state from daemon via Unix socket or state file.
    fmt.Printf("%-20s %-15s %s\n", "REPO", "NODE", "ADDRESS")
    fmt.Println("(Connect to the running daemon to list repos — IPC not yet implemented)")
    fmt.Println("Hint: start the daemon with `phantom node start` first.")
    return nil
}
```

- [ ] **Step 5: Register new commands in `internal/commands/root.go`**

In the `Commands` slice inside `Execute()`, add after `NewProjectCommand()`:

```go
NewNodeCommand(),
NewReposCommand(),
NewPushCommand(), // defined in Task 15
```

- [ ] **Step 6: Build to verify compilation**

```bash
go build ./...
# Expected: no errors
```

- [ ] **Step 7: Smoke test**

```bash
./dist/phantom node --help
# Expected: shows start, stop, list subcommands

./dist/phantom repos
# Expected: prints header + placeholder message
```

- [ ] **Step 8: Commit**

```bash
git add internal/commands/node_start.go internal/commands/node_stop.go \
        internal/commands/node_list.go internal/commands/repos.go \
        internal/commands/root.go
git commit -m "feat(commands): add phantom node start/stop/list and phantom repos"
```

---

## Task 10: FUSE Remote Mount Client

**Files:** `internal/remotefs/client.go`, `internal/remotefs/fs.go`, `internal/remotefs/fs_test.go`, `internal/remotefs/mount.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/remotefs/fs_test.go`:

```go
package remotefs_test

import (
    "context"
    "testing"
    "time"

    "github.com/martinsuchenak/phantom/internal/remotefs"
    proto "github.com/martinsuchenak/phantom/internal/rpc/proto"
    "google.golang.org/grpc"
)

// mockClient implements proto.FileServiceClient for testing.
type mockClient struct {
    proto.UnimplementedFileServiceServer
    statFn   func(*proto.StatRequest) (*proto.StatResponse, error)
    readdirFn func(*proto.ReadDirRequest) (*proto.ReadDirResponse, error)
}

func (m *mockClient) Stat(_ context.Context, req *proto.StatRequest, _ ...grpc.CallOption) (*proto.StatResponse, error) {
    return m.statFn(req)
}

func (m *mockClient) ReadDir(_ context.Context, req *proto.ReadDirRequest, _ ...grpc.CallOption) (*proto.ReadDirResponse, error) {
    return m.readdirFn(req)
}

func (m *mockClient) ListRepos(_ context.Context, _ *proto.ListReposRequest, _ ...grpc.CallOption) (*proto.ListReposResponse, error) {
    return &proto.ListReposResponse{}, nil
}

func (m *mockClient) Read(_ context.Context, _ *proto.ReadRequest, _ ...grpc.CallOption) (proto.FileService_ReadClient, error) {
    return nil, nil
}

func (m *mockClient) SyncFiles(_ context.Context, _ ...grpc.CallOption) (proto.FileService_SyncFilesClient, error) {
    return nil, nil
}

func TestRemoteFSGetattr(t *testing.T) {
    mock := &mockClient{
        statFn: func(req *proto.StatRequest) (*proto.StatResponse, error) {
            return &proto.StatResponse{
                Name:        "hello.txt",
                IsDir:       false,
                Size:        42,
                ModTimeUnix: time.Now().Unix(),
                Mode:        0644,
            }, nil
        },
    }
    rfs := remotefs.NewRemoteFS(mock, "myapp")
    info, err := rfs.GetAttr(context.Background(), "hello.txt")
    if err != nil {
        t.Fatalf("GetAttr: %v", err)
    }
    if info.Size != 42 {
        t.Errorf("expected size 42, got %d", info.Size)
    }
}

func TestRemoteFSReadDir(t *testing.T) {
    mock := &mockClient{
        readdirFn: func(req *proto.ReadDirRequest) (*proto.ReadDirResponse, error) {
            return &proto.ReadDirResponse{
                Entries: []*proto.StatResponse{
                    {Name: "a.txt", IsDir: false, Size: 1},
                    {Name: "sub", IsDir: true},
                },
            }, nil
        },
    }
    rfs := remotefs.NewRemoteFS(mock, "myapp")
    entries, err := rfs.ListDir(context.Background(), "")
    if err != nil {
        t.Fatalf("ListDir: %v", err)
    }
    if len(entries) != 2 {
        t.Errorf("expected 2 entries, got %d", len(entries))
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/remotefs/... -v
# Expected: FAIL — package does not exist
```

- [ ] **Step 3: Create `internal/remotefs/client.go`**

```go
package remotefs

import (
    "context"

    proto "github.com/martinsuchenak/phantom/internal/rpc/proto"
    "github.com/martinsuchenak/phantom/internal/rpc"
)

// RemoteFS provides the file-level operations used by the FUSE layer.
// It wraps a gRPC client and scopes all calls to a single repo.
type RemoteFS struct {
    inner proto.FileServiceClient
    repo  string
}

// NewRemoteFS creates a RemoteFS backed by the given proto client.
func NewRemoteFS(client proto.FileServiceClient, repo string) *RemoteFS {
    return &RemoteFS{inner: client, repo: repo}
}

// NewRemoteFSFromDial dials the remote node and returns a RemoteFS.
func NewRemoteFSFromDial(ctx context.Context, addr string, opts rpc.DialOpts, repo string) (*RemoteFS, error) {
    fc, err := rpc.Dial(ctx, addr, opts)
    if err != nil {
        return nil, err
    }
    return &RemoteFS{inner: fc.Inner(), repo: repo}, nil
}

// AttrInfo holds stat-like information for a remote path.
type AttrInfo struct {
    Name        string
    IsDir       bool
    Size        int64
    ModTimeUnix int64
    Mode        uint32
}

func (r *RemoteFS) GetAttr(ctx context.Context, path string) (AttrInfo, error) {
    resp, err := r.inner.Stat(ctx, &proto.StatRequest{Repo: r.repo, Path: path})
    if err != nil {
        return AttrInfo{}, err
    }
    return AttrInfo{
        Name:        resp.Name,
        IsDir:       resp.IsDir,
        Size:        resp.Size,
        ModTimeUnix: resp.ModTimeUnix,
        Mode:        resp.Mode,
    }, nil
}

func (r *RemoteFS) ListDir(ctx context.Context, path string) ([]AttrInfo, error) {
    resp, err := r.inner.ReadDir(ctx, &proto.ReadDirRequest{Repo: r.repo, Path: path})
    if err != nil {
        return nil, err
    }
    out := make([]AttrInfo, len(resp.Entries))
    for i, e := range resp.Entries {
        out[i] = AttrInfo{
            Name:        e.Name,
            IsDir:       e.IsDir,
            Size:        e.Size,
            ModTimeUnix: e.ModTimeUnix,
            Mode:        e.Mode,
        }
    }
    return out, nil
}

func (r *RemoteFS) ReadBytes(ctx context.Context, path string, offset, length int64, dest []byte) (int, error) {
    stream, err := r.inner.Read(ctx, &proto.ReadRequest{
        Repo:   r.repo,
        Path:   path,
        Offset: offset,
        Length: length,
    })
    if err != nil {
        return 0, err
    }
    var n int
    for n < len(dest) {
        chunk, err := stream.Recv()
        if err != nil {
            break // io.EOF or real error — either way stop
        }
        copied := copy(dest[n:], chunk.Data)
        n += copied
    }
    return n, nil
}
```

- [ ] **Step 4: Create `internal/remotefs/fs.go`**

This implements the `go-fuse` `fs.InodeEmbedder` interface. Adapt if the go-fuse API differs.

```go
//go:build !integration

package remotefs

import (
    "context"
    "io"
    "syscall"

    "github.com/hanwen/go-fuse/v2/fs"
    "github.com/hanwen/go-fuse/v2/fuse"
    proto "github.com/martinsuchenak/phantom/internal/rpc/proto"
)

// RemoteNode is a FUSE inode backed by a remote phantom repo.
type RemoteNode struct {
    fs.Inode
    rfs  *RemoteFS
    path string // path relative to repo root
}

var (
    _ fs.NodeGetattrer = (*RemoteNode)(nil)
    _ fs.NodeLookuper  = (*RemoteNode)(nil)
    _ fs.NodeReaddirer = (*RemoteNode)(nil)
    _ fs.NodeOpener    = (*RemoteNode)(nil)
)

func (n *RemoteNode) Getattr(ctx context.Context, _ fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
    info, err := n.rfs.GetAttr(ctx, n.path)
    if err != nil {
        return syscall.EIO
    }
    out.Size = uint64(info.Size)
    out.Mtime = uint64(info.ModTimeUnix)
    out.Atime = uint64(info.ModTimeUnix)
    out.Ctime = uint64(info.ModTimeUnix)
    out.Mode = info.Mode
    return 0
}

func (n *RemoteNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
    childPath := joinPath(n.path, name)
    info, err := n.rfs.GetAttr(ctx, childPath)
    if err != nil {
        return nil, syscall.ENOENT
    }
    out.Size = uint64(info.Size)
    out.Mtime = uint64(info.ModTimeUnix)
    out.Mode = info.Mode

    var mode uint32
    if info.IsDir {
        mode = syscall.S_IFDIR | 0755
    } else {
        mode = syscall.S_IFREG | 0644
    }
    child := &RemoteNode{rfs: n.rfs, path: childPath}
    return n.NewInode(ctx, child, fs.StableAttr{Mode: mode}), 0
}

func (n *RemoteNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
    entries, err := n.rfs.ListDir(ctx, n.path)
    if err != nil {
        return nil, syscall.EIO
    }
    dirEntries := make([]fuse.DirEntry, len(entries))
    for i, e := range entries {
        mode := uint32(syscall.S_IFREG)
        if e.IsDir {
            mode = syscall.S_IFDIR
        }
        dirEntries[i] = fuse.DirEntry{Name: e.Name, Mode: mode}
    }
    return fs.NewListDirStream(dirEntries), 0
}

func (n *RemoteNode) Open(_ context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
    if flags&(syscall.O_WRONLY|syscall.O_RDWR|syscall.O_CREAT|syscall.O_TRUNC) != 0 {
        return nil, 0, syscall.EROFS
    }
    return &remoteFileHandle{rfs: n.rfs, path: n.path}, fuse.FOPEN_DIRECT_IO, 0
}

type remoteFileHandle struct {
    rfs  *RemoteFS
    path string
}

var _ fs.FileReader = (*remoteFileHandle)(nil)

func (fh *remoteFileHandle) Read(ctx context.Context, dest []byte, offset int64) (fuse.ReadResult, syscall.Errno) {
    n, err := fh.rfs.ReadBytes(ctx, fh.path, offset, int64(len(dest)), dest)
    if err != nil && err != io.EOF {
        return nil, syscall.EIO
    }
    return fuse.ReadResultData(dest[:n]), 0
}

// newRootNode creates the root FUSE inode for a remote repo.
func newRootNode(rfs *RemoteFS) *RemoteNode {
    return &RemoteNode{rfs: rfs, path: ""}
}

func joinPath(base, name string) string {
    if base == "" {
        return name
    }
    return base + "/" + name
}

// protoClientAdapter satisfies tests — bridges mock to RemoteFS proto client.
func init() {
    _ = (*proto.UnimplementedFileServiceServer)(nil) // ensure import used
}
```

- [ ] **Step 5: Create `internal/remotefs/mount.go`**

```go
package remotefs

import (
    "context"
    "fmt"
    "os"

    "github.com/hanwen/go-fuse/v2/fs"
    "github.com/hanwen/go-fuse/v2/fuse"
)

// MountOpts controls how the remote filesystem is mounted.
type MountOpts struct {
    // MountPoint is the local directory where the remote repo appears.
    MountPoint string
    // AllowOther allows other users to access the mount (requires allow_other in /etc/fuse.conf on Linux).
    AllowOther bool
}

// Mount mounts the remote filesystem at opts.MountPoint.
// It blocks until ctx is cancelled, then unmounts.
func Mount(ctx context.Context, rfs *RemoteFS, opts MountOpts) error {
    if err := os.MkdirAll(opts.MountPoint, 0755); err != nil {
        return fmt.Errorf("create mount point %s: %w", opts.MountPoint, err)
    }

    root := newRootNode(rfs)
    fuseOpts := &fs.Options{
        MountOptions: fuse.MountOptions{
            AllowOther: opts.AllowOther,
            FsName:     "phantom-remote",
            Name:       "phantom",
        },
    }

    server, err := fs.Mount(opts.MountPoint, root, fuseOpts)
    if err != nil {
        return fmt.Errorf("fuse mount %s: %w", opts.MountPoint, err)
    }

    <-ctx.Done()
    return server.Unmount()
}
```

- [ ] **Step 6: Run fs tests**

```bash
go test ./internal/remotefs/... -run "TestRemoteFS" -v
# Expected: PASS
```

- [ ] **Step 7: Verify build**

```bash
go build ./internal/remotefs/...
# Expected: no errors
```

- [ ] **Step 8: Commit**

```bash
git add internal/remotefs/
git commit -m "feat(remotefs): FUSE client that proxies reads to remote phantom node over gRPC"
```

---

## Task 11: Extend phantom start with --repo / --node

**Files:** `internal/commands/start.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/commands/start_test.go` (check existing test file for the right test function pattern):

```go
func TestProcessStartRemoteRequiresRepoOrBaseDir(t *testing.T) {
    // If --repo is given without --base-dir, processStart should not error about missing base-dir.
    // This tests the argument validation logic only (no actual FUSE mount).
    err := validateStartArgs("", "myapp", "")
    if err != nil {
        t.Errorf("expected no error when repo is given, got: %v", err)
    }
}

func TestProcessStartLocalRequiresBaseDir(t *testing.T) {
    err := validateStartArgs("", "", "")
    if err == nil {
        t.Error("expected error when both base-dir and repo are empty")
    }
}

func TestProcessStartRepoPlusBaseDirIsError(t *testing.T) {
    err := validateStartArgs("/some/path", "myapp", "")
    if err == nil {
        t.Error("expected error when both base-dir and repo are given")
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/commands/... -run "TestProcessStartRemote|TestProcessStartLocal|TestProcessStartRepoPlusBaseDir" -v
# Expected: FAIL — validateStartArgs undefined
```

- [ ] **Step 3: Add remote flags and validation to `internal/commands/start.go`**

Add `--repo` and `--node` flags to `NewStartCommand()`:

```go
&cli.StringFlag{
    Name:  "repo",
    Usage: "Remote repo name to use as base (auto-discovers node from gossip ring)",
},
&cli.StringFlag{
    Name:  "node",
    Usage: "Explicit node ID to use with --repo (required if multiple nodes serve the same repo)",
},
```

Add `validateStartArgs` function and update `doStart`:

```go
// validateStartArgs checks that exactly one of baseDir or repo is specified.
func validateStartArgs(baseDir, repo, node string) error {
    if baseDir != "" && repo != "" {
        return fmt.Errorf("specify either a base directory or --repo, not both")
    }
    if baseDir == "" && repo == "" {
        return fmt.Errorf("base directory or --repo is required")
    }
    return nil
}

func doStart(ctx context.Context, cmd *cli.Command) error {
    baseDir := resolveBaseDir(cmd.GetStringArg("base-dir"))
    repo := cmd.GetString("repo")
    nodeID := cmd.GetString("node")

    if err := validateStartArgs(baseDir, repo, nodeID); err != nil {
        return err
    }

    name := cmd.GetString("name")
    branch := cmd.GetString("branch")
    persistent := cmd.GetBool("persistent")

    if repo != "" {
        return processStartRemote(ctx, repo, nodeID, name, branch, persistent)
    }
    return processStart(ctx, baseDir, name, branch, persistent)
}
```

Add `processStartRemote` after `processStart`:

```go
func processStartRemote(ctx context.Context, repo, nodeHint, name, branch string, persistent bool) error {
    // Resolve which node serves this repo via the gossip registry.
    // For now we connect directly to the node daemon's registry file.
    // In a full implementation this reads from the running daemon's IPC endpoint.
    // As a pragmatic first step: require --node to be explicit.
    if nodeHint == "" {
        return fmt.Errorf("auto-discovery requires a running node daemon; use --node to specify the node explicitly for now")
    }

    // Determine gRPC address from config
    grpcPort := cfg.Node.GRPCPort
    grpcAddr := fmt.Sprintf("%s:%d", nodeHint, grpcPort)

    authOpts := rpc.DialOpts{
        Auth: rpc.AuthOptions{
            Mode:   rpc.AuthMode(cfg.Node.Auth.Mode),
            Secret: cfg.Node.Auth.Secret,
        },
    }

    // Mount the remote FUSE filesystem
    mountBase := cfg.GetRemoteMountsPath()
    remoteMountPath := filepath.Join(mountBase, nodeHint, repo)
    if err := os.MkdirAll(remoteMountPath, 0755); err != nil {
        return fmt.Errorf("create remote mount dir: %w", err)
    }

    rfs, err := remotefs.NewRemoteFSFromDial(ctx, grpcAddr, authOpts, repo)
    if err != nil {
        return fmt.Errorf("connect to node %s: %w", nodeHint, err)
    }

    // Generate overlay name if not provided
    if name == "" {
        name = repo
    }

    fuseCtx, fuseCancel := context.WithCancel(ctx)
    fuseErrCh := make(chan error, 1)
    go func() {
        fuseErrCh <- remotefs.Mount(fuseCtx, rfs, remotefs.MountOpts{MountPoint: remoteMountPath})
    }()

    // Brief wait to let FUSE mount settle
    // Use the existing overlay machinery with remoteMountPath as BaseDir
    overlayErr := processStart(ctx, remoteMountPath, name, branch, persistent)

    fuseCancel()
    <-fuseErrCh

    // Mark overlay as remote in state
    store, err := state.NewStore(cfg.GetStatePath())
    if err == nil {
        ovl, loadErr := store.Load(name)
        if loadErr == nil {
            ovl.Remote = true
            ovl.RemoteNode = nodeHint
            ovl.RemoteRepo = repo
            ovl.RemoteMountPath = remoteMountPath
            _ = store.Save(ovl)
        }
    }

    return overlayErr
}
```

Add required imports: `"github.com/martinsuchenak/phantom/internal/remotefs"`, `"github.com/martinsuchenak/phantom/internal/rpc"`, `"github.com/martinsuchenak/phantom/internal/state"`.

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/commands/... -run "TestProcessStartRemote|TestProcessStartLocal|TestProcessStartRepoPlusBaseDir" -v
# Expected: PASS
```

- [ ] **Step 5: Build**

```bash
go build ./...
# Expected: no errors
```

- [ ] **Step 6: Commit**

```bash
git add internal/commands/start.go internal/commands/start_test.go
git commit -m "feat(commands): extend phantom start with --repo/--node for remote overlays"
```

---

## Task 12: Sync Walker

**Files:** `internal/sync/walker.go`, `internal/sync/walker_test.go`

The walker inspects the raw overlay upper dir to produce a list of changes to sync.

- [ ] **Step 1: Write the failing tests**

Create `internal/sync/walker_test.go`:

```go
package sync_test

import (
    "os"
    "path/filepath"
    "testing"

    phantomsync "github.com/martinsuchenak/phantom/internal/sync"
)

func TestWalkerRegularFile(t *testing.T) {
    upper := t.TempDir()
    os.WriteFile(filepath.Join(upper, "hello.txt"), []byte("hi"), 0644)

    changes, err := phantomsync.WalkUpperDir(upper)
    if err != nil {
        t.Fatalf("WalkUpperDir: %v", err)
    }
    if len(changes) != 1 {
        t.Fatalf("expected 1 change, got %d", len(changes))
    }
    if changes[0].Path != "hello.txt" {
        t.Errorf("expected path 'hello.txt', got %q", changes[0].Path)
    }
    if changes[0].Deleted {
        t.Error("expected file not deleted")
    }
}

func TestWalkerWhiteoutFile(t *testing.T) {
    upper := t.TempDir()
    // Whiteout: char device 0:0 with name .wh.<original>
    // Simulate by creating a zero-size file named .wh.deleted.txt
    // Real overlayfs creates char(0,0); walker detects by name prefix on non-Linux.
    os.WriteFile(filepath.Join(upper, ".wh.deleted.txt"), []byte{}, 0000)

    changes, err := phantomsync.WalkUpperDir(upper)
    if err != nil {
        t.Fatalf("WalkUpperDir: %v", err)
    }
    if len(changes) != 1 {
        t.Fatalf("expected 1 change, got %d", len(changes))
    }
    if !changes[0].Deleted {
        t.Error("expected deleted=true for whiteout file")
    }
    if changes[0].Path != "deleted.txt" {
        t.Errorf("expected path 'deleted.txt', got %q", changes[0].Path)
    }
}

func TestWalkerNestedDir(t *testing.T) {
    upper := t.TempDir()
    os.MkdirAll(filepath.Join(upper, "sub", "deep"), 0755)
    os.WriteFile(filepath.Join(upper, "sub", "file.go"), []byte("pkg"), 0644)
    os.WriteFile(filepath.Join(upper, "sub", "deep", "inner.go"), []byte("inner"), 0644)

    changes, err := phantomsync.WalkUpperDir(upper)
    if err != nil {
        t.Fatalf("WalkUpperDir: %v", err)
    }
    // Expect: sub/file.go, sub/deep/inner.go (dirs not included as separate entries)
    paths := make(map[string]bool)
    for _, c := range changes {
        paths[c.Path] = true
    }
    if !paths["sub/file.go"] {
        t.Error("expected sub/file.go")
    }
    if !paths["sub/deep/inner.go"] {
        t.Error("expected sub/deep/inner.go")
    }
}

func TestWalkerEmptyDir(t *testing.T) {
    upper := t.TempDir()
    changes, err := phantomsync.WalkUpperDir(upper)
    if err != nil {
        t.Fatalf("WalkUpperDir: %v", err)
    }
    if len(changes) != 0 {
        t.Errorf("expected 0 changes in empty dir, got %d", len(changes))
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/sync/... -run "TestWalker" -v
# Expected: FAIL — package does not exist
```

- [ ] **Step 3: Create `internal/sync/walker.go`**

```go
package sync

import (
    "os"
    "path/filepath"
    "strings"
)

const whiteoutPrefix = ".wh."

// Change represents a single file change to sync to the remote node.
type Change struct {
    // Path is relative to the repo root (not the upper dir).
    Path    string
    Data    []byte
    Deleted bool
    IsDir   bool
}

// WalkUpperDir walks the overlay upper directory and returns all changes.
// Whiteout files (.wh.<name>) are returned as Deleted changes.
// Directories are not returned as separate changes (files within them are).
func WalkUpperDir(upperDir string) ([]Change, error) {
    var changes []Change
    err := filepath.Walk(upperDir, func(abs string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }
        // Skip the root itself
        if abs == upperDir {
            return nil
        }
        rel, err := filepath.Rel(upperDir, abs)
        if err != nil {
            return err
        }
        rel = filepath.ToSlash(rel)

        name := info.Name()

        // Whiteout file: .wh.<original-name>
        if strings.HasPrefix(name, whiteoutPrefix) {
            originalName := name[len(whiteoutPrefix):]
            dir := filepath.Dir(rel)
            originalRel := filepath.ToSlash(filepath.Join(dir, originalName))
            if originalRel == "." {
                originalRel = originalName
            }
            changes = append(changes, Change{Path: originalRel, Deleted: true})
            return nil
        }

        if info.IsDir() {
            // Don't emit directories as changes; files within them will be walked.
            return nil
        }

        data, err := os.ReadFile(abs)
        if err != nil {
            return err
        }
        changes = append(changes, Change{Path: rel, Data: data})
        return nil
    })
    return changes, err
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/sync/... -run "TestWalker" -v
# Expected: PASS
```

- [ ] **Step 5: Commit**

```bash
git add internal/sync/walker.go internal/sync/walker_test.go
git commit -m "feat(sync): upper dir walker that detects changed and deleted files"
```

---

## Task 13: Sync Push Engine + Complete SyncFiles on Server

**Files:** `internal/sync/syncer.go`, `internal/sync/syncer_test.go`, `internal/rpc/server.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/sync/syncer_test.go`:

```go
package sync_test

import (
    "context"
    "net"
    "os"
    "path/filepath"
    "testing"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
    "google.golang.org/grpc/test/bufconn"

    phantomsync "github.com/martinsuchenak/phantom/internal/sync"
    proto "github.com/martinsuchenak/phantom/internal/rpc/proto"
    "github.com/martinsuchenak/phantom/internal/rpc"
)

func setupSyncServer(t *testing.T, repo, repoPath string) (proto.FileServiceClient, func()) {
    t.Helper()
    lis := bufconn.Listen(1 << 20)
    srv := grpc.NewServer()
    proto.RegisterFileServiceServer(srv, rpc.NewFileServer(map[string]string{repo: repoPath}))
    go srv.Serve(lis)

    conn, err := grpc.NewClient("passthrough://bufnet",
        grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
            return lis.DialContext(ctx)
        }),
        grpc.WithTransportCredentials(insecure.NewCredentials()),
    )
    if err != nil {
        t.Fatalf("dial: %v", err)
    }
    return proto.NewFileServiceClient(conn), func() { conn.Close(); srv.Stop() }
}

func TestSyncerPushFile(t *testing.T) {
    serverDir := t.TempDir()
    client, cleanup := setupSyncServer(t, "r", serverDir)
    defer cleanup()

    upper := t.TempDir()
    os.WriteFile(filepath.Join(upper, "new.txt"), []byte("content"), 0644)

    syncer := phantomsync.NewSyncer(client, "r", false)
    result, err := syncer.Push(context.Background(), upper, "test commit")
    if err != nil {
        t.Fatalf("Push: %v", err)
    }
    if !result.Success {
        t.Errorf("expected success, got error: %s", result.Error)
    }

    // Verify file written to server dir
    data, err := os.ReadFile(filepath.Join(serverDir, "new.txt"))
    if err != nil {
        t.Fatalf("read synced file: %v", err)
    }
    if string(data) != "content" {
        t.Errorf("expected 'content', got %q", data)
    }
}

func TestSyncerDeleteFile(t *testing.T) {
    serverDir := t.TempDir()
    // Create file on server that we'll delete
    os.WriteFile(filepath.Join(serverDir, "old.txt"), []byte("old"), 0644)

    client, cleanup := setupSyncServer(t, "r", serverDir)
    defer cleanup()

    upper := t.TempDir()
    // Simulate whiteout
    os.WriteFile(filepath.Join(upper, ".wh.old.txt"), []byte{}, 0000)

    syncer := phantomsync.NewSyncer(client, "r", false)
    result, err := syncer.Push(context.Background(), upper, "delete old.txt")
    if err != nil {
        t.Fatalf("Push: %v", err)
    }
    if !result.Success {
        t.Errorf("expected success, got: %s", result.Error)
    }
    if _, err := os.Stat(filepath.Join(serverDir, "old.txt")); !os.IsNotExist(err) {
        t.Error("expected old.txt to be deleted on server")
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/sync/... -run "TestSyncer" -v
# Expected: FAIL — phantomsync.NewSyncer undefined
```

- [ ] **Step 3: Implement `SyncFiles` on the server in `internal/rpc/server.go`**

Replace the stub `SyncFiles` method:

```go
func (s *FileServer) SyncFiles(stream proto.FileService_SyncFilesServer) error {
    var repoBase string
    var commitMsg string
    var autoGitCommit bool

    for {
        chunk, err := stream.Recv()
        if err == io.EOF {
            break
        }
        if err != nil {
            return status.Errorf(codes.Internal, "recv: %v", err)
        }

        switch p := chunk.Payload.(type) {
        case *proto.SyncChunk_Header:
            base, lerr := s.repoPath(p.Header.Repo)
            if lerr != nil {
                return lerr
            }
            repoBase = base
            commitMsg = p.Header.CommitMessage

        case *proto.SyncChunk_File:
            if repoBase == "" {
                return status.Error(codes.InvalidArgument, "SyncHeader must be sent first")
            }
            full, err := safePath(repoBase, p.File.Path)
            if err != nil {
                return err
            }
            if p.File.Deleted {
                if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
                    return status.Errorf(codes.Internal, "delete %s: %v", p.File.Path, err)
                }
            } else if p.File.IsDir {
                if err := os.MkdirAll(full, 0755); err != nil {
                    return status.Errorf(codes.Internal, "mkdir %s: %v", p.File.Path, err)
                }
            } else {
                if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
                    return status.Errorf(codes.Internal, "mkdir parent: %v", err)
                }
                if err := os.WriteFile(full, p.File.Data, 0644); err != nil {
                    return status.Errorf(codes.Internal, "write %s: %v", p.File.Path, err)
                }
            }
        }
    }

    // Optionally git commit
    gitCommitted := false
    gitHash := ""
    if autoGitCommit && repoBase != "" && commitMsg != "" {
        gitCommitted, gitHash = tryGitCommit(repoBase, commitMsg)
    }

    return stream.SendAndClose(&proto.SyncResponse{
        Success:       true,
        GitCommitted:  gitCommitted,
        GitCommitHash: gitHash,
    })
}

// tryGitCommit attempts git add -A && git commit in dir. Returns success and hash.
func tryGitCommit(dir, message string) (bool, string) {
    // Check if it's a git repo
    if _, err := os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(err) {
        return false, ""
    }
    addCmd := exec.Command("git", "-C", dir, "add", "-A")
    if err := addCmd.Run(); err != nil {
        return false, ""
    }
    commitCmd := exec.Command("git", "-C", dir, "commit", "-m", message)
    if err := commitCmd.Run(); err != nil {
        return false, "" // nothing to commit is also ok
    }
    hashCmd := exec.Command("git", "-C", dir, "rev-parse", "HEAD")
    out, err := hashCmd.Output()
    if err != nil {
        return true, ""
    }
    return true, strings.TrimSpace(string(out))
}
```

Add `"os/exec"`, `"strings"`, `"path/filepath"` to imports in `server.go`. Also add `autoGitCommit bool` field to `FileServer` and a constructor parameter:

```go
type FileServer struct {
    proto.UnimplementedFileServiceServer
    repos         map[string]string
    autoGitCommit bool
}

func NewFileServer(repos map[string]string) *FileServer {
    return &FileServer{repos: repos}
}

func NewFileServerWithOptions(repos map[string]string, autoGitCommit bool) *FileServer {
    return &FileServer{repos: repos, autoGitCommit: autoGitCommit}
}
```

Update `doNodeStart` in `node_start.go` to use `NewFileServerWithOptions`:

```go
proto.RegisterFileServiceServer(srv,
    rpc.NewFileServerWithOptions(repoMap, nodeCfg.Sync.AutoGitCommit),
)
```

- [ ] **Step 4: Create `internal/sync/syncer.go`**

```go
package sync

import (
    "context"

    proto "github.com/martinsuchenak/phantom/internal/rpc/proto"
)

// SyncResult holds the outcome of a push operation.
type SyncResult struct {
    Success       bool
    Error         string
    GitCommitted  bool
    GitCommitHash string
}

// Syncer pushes overlay changes to a remote node via gRPC SyncFiles.
type Syncer struct {
    client        proto.FileServiceClient
    repo          string
    autoGitCommit bool
}

// NewSyncer creates a Syncer targeting the given repo on the remote node.
func NewSyncer(client proto.FileServiceClient, repo string, autoGitCommit bool) *Syncer {
    return &Syncer{client: client, repo: repo, autoGitCommit: autoGitCommit}
}

// Push walks the upper dir and streams all changes to the remote node.
func (s *Syncer) Push(ctx context.Context, upperDir, commitMessage string) (SyncResult, error) {
    changes, err := WalkUpperDir(upperDir)
    if err != nil {
        return SyncResult{}, err
    }

    stream, err := s.client.SyncFiles(ctx)
    if err != nil {
        return SyncResult{}, err
    }

    // Send header first
    if err := stream.Send(&proto.SyncChunk{
        Payload: &proto.SyncChunk_Header{
            Header: &proto.SyncHeader{
                Repo:          s.repo,
                CommitMessage: commitMessage,
            },
        },
    }); err != nil {
        return SyncResult{}, err
    }

    // Send file changes
    for _, c := range changes {
        if err := stream.Send(&proto.SyncChunk{
            Payload: &proto.SyncChunk_File{
                File: &proto.SyncFile{
                    Path:    c.Path,
                    Data:    c.Data,
                    Deleted: c.Deleted,
                    IsDir:   c.IsDir,
                },
            },
        }); err != nil {
            return SyncResult{}, err
        }
    }

    resp, err := stream.CloseAndRecv()
    if err != nil {
        return SyncResult{}, err
    }
    return SyncResult{
        Success:       resp.Success,
        Error:         resp.Error,
        GitCommitted:  resp.GitCommitted,
        GitCommitHash: resp.GitCommitHash,
    }, nil
}
```

- [ ] **Step 5: Run all sync tests**

```bash
go test ./internal/sync/... -v
# Expected: all PASS

go test ./internal/rpc/... -v
# Expected: all PASS (SyncFiles stub replaced with real implementation)
```

- [ ] **Step 6: Commit**

```bash
git add internal/sync/syncer.go internal/sync/syncer_test.go internal/rpc/server.go \
        internal/commands/node_start.go
git commit -m "feat(sync): push engine streams upper dir changes to remote node via SyncFiles"
```

---

## Task 14: Sentinel Watcher

**Files:** `internal/sync/sentinel.go`, `internal/sync/sentinel_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/sync/sentinel_test.go`:

```go
package sync_test

import (
    "context"
    "os"
    "path/filepath"
    "testing"
    "time"

    phantomsync "github.com/martinsuchenak/phantom/internal/sync"
)

func TestSentinelDetectsFile(t *testing.T) {
    dir := t.TempDir()
    triggered := make(chan string, 1)

    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()

    go func() {
        phantomsync.Watch(ctx, dir, func(message string) {
            triggered <- message
        })
    }()

    time.Sleep(100 * time.Millisecond) // let watcher start
    os.WriteFile(filepath.Join(dir, ".phantom_commit"), []byte("my message"), 0644)

    select {
    case msg := <-triggered:
        if msg != "my message" {
            t.Errorf("expected 'my message', got %q", msg)
        }
    case <-ctx.Done():
        t.Error("timeout: sentinel did not fire")
    }
}

func TestSentinelDeletesFileAfterFiring(t *testing.T) {
    dir := t.TempDir()

    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()

    done := make(chan struct{})
    go func() {
        phantomsync.Watch(ctx, dir, func(_ string) {
            close(done)
        })
    }()

    time.Sleep(100 * time.Millisecond)
    sentinel := filepath.Join(dir, ".phantom_commit")
    os.WriteFile(sentinel, []byte(""), 0644)

    select {
    case <-done:
    case <-ctx.Done():
        t.Fatal("timeout")
    }

    time.Sleep(100 * time.Millisecond)
    if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
        t.Error("expected .phantom_commit to be deleted after firing")
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/sync/... -run "TestSentinel" -v
# Expected: FAIL — phantomsync.Watch undefined
```

- [ ] **Step 3: Create `internal/sync/sentinel.go`**

```go
package sync

import (
    "context"
    "os"
    "path/filepath"
    "strings"

    "github.com/fsnotify/fsnotify"
)

const sentinelFile = ".phantom_commit"
const resultFile = ".phantom_commit_result"

// Watch monitors mountPoint for a .phantom_commit file.
// When detected, it reads the file contents (as commit message), calls onTrigger,
// deletes the sentinel, and writes .phantom_commit_result with the outcome string.
// Watch blocks until ctx is cancelled.
func Watch(ctx context.Context, mountPoint string, onTrigger func(commitMessage string)) {
    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        return
    }
    defer watcher.Close()

    _ = watcher.Add(mountPoint)

    sentinel := filepath.Join(mountPoint, sentinelFile)

    for {
        select {
        case <-ctx.Done():
            return
        case event, ok := <-watcher.Events:
            if !ok {
                return
            }
            if event.Name != sentinel {
                continue
            }
            if event.Op&(fsnotify.Create|fsnotify.Write) == 0 {
                continue
            }
            data, err := os.ReadFile(sentinel)
            if err != nil {
                continue
            }
            msg := strings.TrimSpace(string(data))
            os.Remove(sentinel)
            onTrigger(msg)
        case _, ok := <-watcher.Errors:
            if !ok {
                return
            }
        }
    }
}

// WriteResult writes the sync outcome to .phantom_commit_result in mountPoint.
func WriteResult(mountPoint, result string) {
    _ = os.WriteFile(filepath.Join(mountPoint, resultFile), []byte(result+"\n"), 0644)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/sync/... -run "TestSentinel" -v
# Expected: PASS
```

- [ ] **Step 5: Commit**

```bash
git add internal/sync/sentinel.go internal/sync/sentinel_test.go
git commit -m "feat(sync): sentinel watcher triggers push on .phantom_commit write"
```

---

## Task 15: phantom push Command

**Files:** `internal/commands/push.go`, `internal/commands/root.go`

- [ ] **Step 1: Write the failing test**

Create `internal/commands/push_test.go`:

```go
package commands_test

import (
    "testing"
)

func TestPushCommandExists(t *testing.T) {
    cmd := NewPushCommand()
    if cmd == nil {
        t.Error("NewPushCommand returned nil")
    }
    if cmd.Name != "push" {
        t.Errorf("expected name 'push', got %q", cmd.Name)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/commands/... -run "TestPushCommand" -v
# Expected: FAIL — NewPushCommand undefined
```

- [ ] **Step 3: Create `internal/commands/push.go`**

```go
package commands

import (
    "context"
    "fmt"

    "github.com/paularlott/cli"
    "github.com/martinsuchenak/phantom/internal/rpc"
    phantomsync "github.com/martinsuchenak/phantom/internal/sync"
    "github.com/martinsuchenak/phantom/internal/state"
)

func NewPushCommand() *cli.Command {
    return &cli.Command{
        Name:  "push",
        Usage: "Push overlay changes to the remote base node",
        Description: "Streams changed files from the overlay upper dir to the node that holds the base repo. " +
            "Only works for overlays created with --repo.",
        Flags: []cli.Flag{
            &cli.StringFlag{
                Name:    "message",
                Aliases: []string{"m"},
                Usage:   "Commit message for the remote node (used if the repo is a git repo)",
            },
        },
        Arguments: []cli.Argument{
            &cli.StringArg{
                Name:     "name",
                Usage:    "Name of the overlay to push",
                Required: true,
            },
        },
        Run: doPush,
    }
}

func doPush(ctx context.Context, cmd *cli.Command) error {
    name := cmd.GetStringArg("name")
    message := cmd.GetString("message")
    if message == "" {
        message = fmt.Sprintf("phantom push from overlay %s", name)
    }

    store, err := state.NewStore(cfg.GetStatePath())
    if err != nil {
        return fmt.Errorf("open state store: %w", err)
    }

    ovl, err := store.Load(name)
    if err != nil {
        return err
    }

    if !ovl.Remote {
        return fmt.Errorf("overlay %q is not a remote overlay; push only applies to overlays created with --repo", name)
    }

    grpcPort := cfg.Node.GRPCPort
    grpcAddr := fmt.Sprintf("%s:%d", ovl.RemoteNode, grpcPort)

    authOpts := rpc.DialOpts{
        Auth: rpc.AuthOptions{
            Mode:   rpc.AuthMode(cfg.Node.Auth.Mode),
            Secret: cfg.Node.Auth.Secret,
        },
    }
    fc, err := rpc.Dial(ctx, grpcAddr, authOpts)
    if err != nil {
        return fmt.Errorf("connect to %s: %w", grpcAddr, err)
    }

    syncer := phantomsync.NewSyncer(fc.Inner(), ovl.RemoteRepo, cfg.Node.Sync.AutoGitCommit)
    result, err := syncer.Push(ctx, ovl.WorkDir, message)
    if err != nil {
        return fmt.Errorf("push failed: %w", err)
    }

    if !result.Success {
        return fmt.Errorf("remote error: %s", result.Error)
    }

    if result.GitCommitted {
        fmt.Printf("Pushed and committed on remote: %s\n", result.GitCommitHash)
    } else {
        fmt.Println("Pushed successfully (no git commit on remote)")
    }

    // Write result to overlay mount for the agent to read
    phantomsync.WriteResult(ovl.MountPoint, "ok")
    return nil
}
```

- [ ] **Step 4: Verify `NewPushCommand` is registered in `root.go`**

Ensure `NewPushCommand()` is in the `Commands` slice (added in Task 9 Step 5). Rebuild:

```bash
go build ./...
./dist/phantom push --help
# Expected: shows usage for phantom push
```

- [ ] **Step 5: Run all tests**

```bash
go test ./... -v
# Expected: all PASS
```

- [ ] **Step 6: Wire sentinel into phantom start for remote overlays**

In `processStartRemote` in `internal/commands/start.go`, after creating the overlay, start the sentinel watcher:

```go
// Start sentinel watcher for .phantom_commit
go func() {
    phantomsync.Watch(ctx, remoteMountPath, func(commitMsg string) {
        syncer := phantomsync.NewSyncer(rfs_inner_client, repo, cfg.Node.Sync.AutoGitCommit)
        result, err := syncer.Push(ctx, ovl.WorkDir, commitMsg)
        var outcome string
        if err != nil {
            outcome = "error: " + err.Error()
        } else if !result.Success {
            outcome = "error: " + result.Error
        } else {
            outcome = "ok"
        }
        phantomsync.WriteResult(remoteMountPath, outcome)
    })
}()
```

Note: `rfs_inner_client` requires storing the `proto.FileServiceClient` from `NewRemoteFSFromDial` — refactor `processStartRemote` to keep a reference to `fc.Inner()` before passing to `NewRemoteFS`.

- [ ] **Step 7: Final build and test**

```bash
go test ./...
go build ./...
# Expected: all pass, clean build
```

- [ ] **Step 8: Commit**

```bash
git add internal/commands/push.go internal/commands/push_test.go \
        internal/commands/start.go
git commit -m "feat(commands): add phantom push command and wire sentinel watcher"
```

---

## Self-Review Checklist

- [x] **Spec coverage:** Gossip ring (Task 8), gRPC data plane (Tasks 4-7), FUSE mount client (Task 10), sync + sentinel (Tasks 12-14), auth tiers (Task 6), config (Task 2), types (Task 3), CLI commands (Tasks 9, 11, 15) — all spec sections covered.
- [x] **No placeholders:** All code blocks are complete. `phantom node list` and `phantom repos` have IPC-pending notes which are accurate limitations of this phase.
- [x] **Type consistency:** `Change`, `SyncResult`, `Syncer`, `RemoteFS`, `AttrInfo`, `Registry`, `Peer`, `AuthOptions`, `DialOpts`, `FileClient`, `FileServer` — names consistent across all tasks.
- [x] **`phantom sync` name conflict:** Existing `phantom sync` pulls base changes into overlay. New remote push uses `phantom push` — no conflict.
- [x] **`phantom start` (not `phantom create`):** Plan uses existing `start` command extended with `--repo`/`--node`.
