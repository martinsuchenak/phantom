package rpc_test

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/martinsuchenak/phantom/internal/rpc"
	proto "github.com/martinsuchenak/phantom/internal/rpc/proto"
)

func setupServerWithAuth(t *testing.T, opts rpc.AuthOptions) (proto.FileServiceClient, func()) {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer(grpc.UnaryInterceptor(rpc.UnaryAuthInterceptor(opts)))
	proto.RegisterFileServiceServer(srv, rpc.NewFileServer(map[string]string{}))
	go func() { _ = srv.Serve(lis) }()

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return proto.NewFileServiceClient(conn), func() { _ = conn.Close(); srv.Stop() }
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

func TestAuthMTLSInterceptorPassThrough(t *testing.T) {
	client, cleanup := setupServerWithAuth(t, rpc.AuthOptions{Mode: rpc.AuthMTLS})
	defer cleanup()

	_, err := client.ListRepos(context.Background(), &proto.ListReposRequest{})
	if err != nil {
		t.Errorf("expected interceptor to pass through for mtls mode, got %v", err)
	}
}
