package rpc

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	proto "github.com/martinsuchenak/phantom/internal/rpc/proto"
)

const chunkSize = 64 * 1024

type FileServer struct {
	proto.UnimplementedFileServiceServer
	mu               sync.RWMutex
	repos            map[string]string
	autoGitCommit    bool
	maxFileSizeBytes int64
}

func NewFileServer(repos map[string]string) *FileServer {
	return &FileServer{repos: repos}
}

func NewFileServerWithOptions(repos map[string]string, autoGitCommit bool, maxFileSizeBytes int64) *FileServer {
	return &FileServer{repos: repos, autoGitCommit: autoGitCommit, maxFileSizeBytes: maxFileSizeBytes}
}

// UpdateRepos replaces the served repo map at runtime (live reload).
func (s *FileServer) UpdateRepos(repos map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.repos = repos
}

func (s *FileServer) repoPath(repo string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	path, ok := s.repos[repo]
	if !ok {
		return "", status.Errorf(codes.NotFound, "repo %q not found", repo)
	}
	return path, nil
}

func safePath(base, rel string) (string, error) {
	joined := filepath.Join(base, filepath.Clean("/"+rel))
	if !strings.HasPrefix(joined, base+string(filepath.Separator)) && joined != base {
		return "", status.Error(codes.InvalidArgument, "path traversal denied")
	}
	return joined, nil
}

func (s *FileServer) ListRepos(_ context.Context, _ *proto.ListReposRequest) (*proto.ListReposResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	infos := make([]*proto.RepoInfo, 0, len(s.repos))
	for name := range s.repos {
		infos = append(infos, &proto.RepoInfo{Name: name})
	}
	return &proto.ListReposResponse{Repos: infos}, nil
}

// unixMode converts a Go os.FileMode to a POSIX mode_t value suitable for
// use in FUSE attr responses. Go's FileMode bit positions differ from POSIX:
// e.g. ModeDir = 1<<31, whereas S_IFDIR = 0x4000.
func unixMode(m os.FileMode) uint32 {
	out := uint32(m.Perm())
	switch {
	case m.IsDir():
		out |= syscall.S_IFDIR
	case m&os.ModeSymlink != 0:
		out |= syscall.S_IFLNK
	case m&os.ModeDevice != 0:
		if m&os.ModeCharDevice != 0 {
			out |= syscall.S_IFCHR
		} else {
			out |= syscall.S_IFBLK
		}
	case m&os.ModeNamedPipe != 0:
		out |= syscall.S_IFIFO
	case m&os.ModeSocket != 0:
		out |= syscall.S_IFSOCK
	default:
		out |= syscall.S_IFREG
	}
	if m&os.ModeSetuid != 0 {
		out |= syscall.S_ISUID
	}
	if m&os.ModeSetgid != 0 {
		out |= syscall.S_ISGID
	}
	if m&os.ModeSticky != 0 {
		out |= syscall.S_ISVTX
	}
	return out
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
		Mode:        unixMode(info.Mode()),
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
			Mode:        unixMode(info.Mode()),
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
	defer func() { _ = f.Close() }()

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
	var repoBase string
	var commitMsg string

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
			if !p.File.Deleted && !p.File.IsDir && s.maxFileSizeBytes > 0 && int64(len(p.File.Data)) > s.maxFileSizeBytes {
				return status.Errorf(codes.InvalidArgument, "file %s exceeds maximum allowed size", p.File.Path)
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

	gitCommitted := false
	gitHash := ""
	if s.autoGitCommit && repoBase != "" && commitMsg != "" {
		gitCommitted, gitHash = tryGitCommit(repoBase, commitMsg)
	}

	return stream.SendAndClose(&proto.SyncResponse{
		Success:       true,
		GitCommitted:  gitCommitted,
		GitCommitHash: gitHash,
	})
}

func tryGitCommit(dir, message string) (bool, string) {
	if _, err := os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(err) {
		return false, ""
	}
	addCmd := exec.Command("git", "-C", dir, "add", "-A")
	if err := addCmd.Run(); err != nil {
		return false, ""
	}
	commitCmd := exec.Command("git", "-C", dir, "commit", "-m", message)
	if err := commitCmd.Run(); err != nil {
		return false, ""
	}
	hashCmd := exec.Command("git", "-C", dir, "rev-parse", "HEAD")
	out, err := hashCmd.Output()
	if err != nil {
		return true, ""
	}
	return true, strings.TrimSpace(string(out))
}
