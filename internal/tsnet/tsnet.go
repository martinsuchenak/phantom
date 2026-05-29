package tsnet

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"tailscale.com/tsnet"
)

type acceptResult struct {
	conn net.Conn
	err  error
}

type DualListener struct {
	local    net.Listener
	tsnet    net.Listener
	acceptCh chan acceptResult
	closed   chan struct{}
	once     sync.Once
	wg       sync.WaitGroup
}

func NewDualListener(local, ts net.Listener) *DualListener {
	dl := &DualListener{
		local:    local,
		tsnet:    ts,
		acceptCh: make(chan acceptResult),
		closed:   make(chan struct{}),
	}
	dl.wg.Add(2)
	go dl.acceptLoop(dl.local)
	go dl.acceptLoop(dl.tsnet)
	return dl
}

func (d *DualListener) acceptLoop(l net.Listener) {
	defer d.wg.Done()
	for {
		conn, err := l.Accept()
		select {
		case <-d.closed:
			if conn != nil {
				_ = conn.Close()
			}
			return
		case d.acceptCh <- acceptResult{conn: conn, err: err}:
			if err != nil {
				return
			}
		}
	}
}

func (d *DualListener) Accept() (net.Conn, error) {
	r, ok := <-d.acceptCh
	if !ok {
		return nil, fmt.Errorf("listener closed")
	}
	return r.conn, r.err
}

func (d *DualListener) Close() error {
	d.once.Do(func() {
		close(d.closed)
		_ = d.local.Close()
		_ = d.tsnet.Close()
		d.wg.Wait()
		close(d.acceptCh)
	})
	return nil
}

func (d *DualListener) Addr() net.Addr {
	return d.local.Addr()
}

type SmartDialer struct {
	server              *tsnet.Server
	fallbackDialTimeout time.Duration
}

func NewSmartDialer(server *tsnet.Server, fallbackTimeout time.Duration) *SmartDialer {
	return &SmartDialer{server: server, fallbackDialTimeout: fallbackTimeout}
}

func (t *SmartDialer) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(host)
	if ip != nil && IsCGNAT(ip) {
		return t.server.Dial(ctx, network, addr)
	}
	return net.DialTimeout(network, addr, t.fallbackDialTimeout)
}

func (t *SmartDialer) DialContext(ctx context.Context, addr string) (net.Conn, error) {
	return t.Dial(ctx, "tcp", addr)
}

func IsCGNAT(ip net.IP) bool {
	_, cgnat, _ := net.ParseCIDR("100.64.0.0/10")
	return cgnat.Contains(ip)
}

type Config struct {
	Hostname   string
	Dir        string
	AuthKey    string
	ControlURL string
}

func IsEnabled(cfg Config) bool {
	return cfg.Hostname != ""
}

func Setup(ctx context.Context, cfg Config, bindAddr string) (*tsnet.Server, *DualListener, error) {
	srv := &tsnet.Server{
		Hostname:   cfg.Hostname,
		Dir:        cfg.Dir,
		AuthKey:    cfg.AuthKey,
		ControlURL: cfg.ControlURL,
		Ephemeral:  cfg.AuthKey != "",
	}
	if err := srv.Start(); err != nil {
		return nil, nil, fmt.Errorf("tsnet start: %w", err)
	}

	status, err := srv.Up(ctx)
	if err != nil {
		_ = srv.Close()
		return nil, nil, fmt.Errorf("tsnet up: %w", err)
	}

	tsListener, err := srv.Listen("tcp", bindAddr)
	if err != nil {
		_ = srv.Close()
		return nil, nil, fmt.Errorf("tsnet listen: %w", err)
	}

	localListener, err := net.Listen("tcp", bindAddr)
	if err != nil {
		_ = tsListener.Close()
		_ = srv.Close()
		return nil, nil, fmt.Errorf("local listen: %w", err)
	}

	dl := NewDualListener(localListener, tsListener)

	var tailscaleIPs []string
	for _, ip := range status.TailscaleIPs {
		tailscaleIPs = append(tailscaleIPs, ip.String())
	}

	_ = tailscaleIPs
	return srv, dl, nil
}
