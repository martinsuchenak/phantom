package tsnet

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestIsCGNAT(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"100.64.0.1", true},
		{"100.127.255.254", true},
		{"100.64.1.2", true},
		{"100.100.100.100", true},
		{"10.0.0.1", false},
		{"192.168.1.1", false},
		{"172.16.0.1", false},
		{"127.0.0.1", false},
		{"8.8.8.8", false},
		{"192.168.1.10", false},
	}
	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			got := IsCGNAT(net.ParseIP(tt.ip))
			if got != tt.want {
				t.Errorf("IsCGNAT(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestDualListenerAcceptFromLocal(t *testing.T) {
	local, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("local listen: %v", err)
	}

	fakeRemote, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("fake remote listen: %v", err)
	}

	dl := NewDualListener(local, fakeRemote)
	defer func() { _ = dl.Close() }()

	conn, err := net.Dial("tcp", local.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = conn.Close()

	accepted, err := dl.Accept()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	_ = accepted.Close()
}

func TestDualListenerClose(t *testing.T) {
	local, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fakeRemote, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	dl := NewDualListener(local, fakeRemote)

	if err := dl.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if err := dl.Close(); err != nil {
		t.Fatalf("second close should be no-op: %v", err)
	}

	_, err = dl.Accept()
	if err == nil {
		t.Error("expected error from closed listener")
	}
}

func TestDualListenerAddr(t *testing.T) {
	local, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fakeRemote, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = local.Close() }()
	defer func() { _ = fakeRemote.Close() }()

	dl := NewDualListener(local, fakeRemote)
	defer func() { _ = dl.Close() }()

	if dl.Addr().String() != local.Addr().String() {
		t.Errorf("expected Addr() to return local addr %s, got %s", local.Addr(), dl.Addr())
	}
}

func TestIsEnabled(t *testing.T) {
	if IsEnabled(Config{}) {
		t.Error("expected empty config to be disabled")
	}
	if !IsEnabled(Config{Hostname: "phantom-node"}) {
		t.Error("expected non-empty hostname to be enabled")
	}
}

func TestSmartDialerNonCGNATUsesFallback(t *testing.T) {
	server, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.Close() }()

	dialer := NewSmartDialer(nil, 5*time.Second)
	ctx := context.Background()

	conn, err := dialer.Dial(ctx, "tcp", server.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = conn.Close()
}
