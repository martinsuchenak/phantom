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

type AuthOptions struct {
	Mode   AuthMode
	Secret string
}

func UnaryAuthInterceptor(opts AuthOptions) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := checkAuth(ctx, opts); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

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
		return nil
	default:
		return status.Errorf(codes.Internal, "unknown auth mode %q", opts.Mode)
	}
}

func UnaryClientSecretInterceptor(secret string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = metadata.AppendToOutgoingContext(ctx, "phantom-secret", secret)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func StreamClientSecretInterceptor(secret string) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		ctx = metadata.AppendToOutgoingContext(ctx, "phantom-secret", secret)
		return streamer(ctx, desc, cc, method, opts...)
	}
}
