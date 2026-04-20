package sync

import (
	"context"
	"fmt"

	proto "github.com/martinsuchenak/phantom/internal/rpc/proto"
)

type SyncResult struct {
	Success       bool
	Error         string
	GitCommitted  bool
	GitCommitHash string
}

type Syncer struct {
	client           proto.FileServiceClient
	repo             string
	maxFileSizeBytes int64
}

func NewSyncer(client proto.FileServiceClient, repo string, maxFileSizeBytes int64) *Syncer {
	return &Syncer{client: client, repo: repo, maxFileSizeBytes: maxFileSizeBytes}
}

func (s *Syncer) Push(ctx context.Context, upperDir, commitMessage string) (SyncResult, error) {
	changes, err := WalkUpperDir(upperDir, s.maxFileSizeBytes)
	if err != nil {
		return SyncResult{}, fmt.Errorf("walk upper dir: %w", err)
	}

	if len(changes) == 0 {
		return SyncResult{Success: true}, nil
	}

	stream, err := s.client.SyncFiles(ctx)
	if err != nil {
		return SyncResult{}, err
	}

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
