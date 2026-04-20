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

type FileInfo struct {
	Name        string
	IsDir       bool
	Size        int64
	ModTimeUnix int64
	Mode        uint32
}

type FileClient struct {
	inner proto.FileServiceClient
}

func NewFileClient(inner proto.FileServiceClient) *FileClient {
	return &FileClient{inner: inner}
}

type DialOpts struct {
	Auth   AuthOptions
	CAFile string
	Cert   string
	Key    string
}

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

func (c *FileClient) Inner() proto.FileServiceClient {
	return c.inner
}
