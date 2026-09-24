package sshlib

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// TunnelRule defines a port forwarding or proxy rule.
type TunnelRule struct {
	Type        string `json:"type"`                  // "local" (-L), "remote" (-R), "dynamic" (-D)
	Local       string `json:"local"`                 // e.g. "127.0.0.1:5900", ":5900", "127.0.0.1:1080"
	Remote      string `json:"remote,omitempty"`      // e.g. "10.20.13.115:5900", "localhost:3000"
	Description string `json:"description,omitempty"` // e.g. "VNC Desktop", "NewAPI", "SOCKS5 Proxy"
	AutoStart   bool   `json:"autoStart,omitempty"`   // automatically start with SSH connection
}

// Normalize ensures local and remote addresses have proper host/port defaults.
func (r *TunnelRule) Normalize() {
	r.Type = strings.ToLower(strings.TrimSpace(r.Type))
	if r.Type == "" {
		r.Type = "local"
	}
	r.Local = strings.TrimSpace(r.Local)
	if !strings.Contains(r.Local, ":") {
		r.Local = "127.0.0.1:" + r.Local
	}
	r.Remote = strings.TrimSpace(r.Remote)
}

// ActiveTunnel represents a running tunnel instance.
type ActiveTunnel struct {
	Rule       TunnelRule
	Listener   io.Closer
	ActiveConn int64
	BytesIn    int64
	BytesOut   int64
	Status     string // "active", "error", "closed"
	Error      error
	cancel     context.CancelFunc
}

// TunnelManager coordinates running tunnels.
type TunnelManager struct {
	mu      sync.Mutex
	tunnels []*ActiveTunnel
	client  *Client
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewTunnelManager creates a new tunnel manager for an SSH client.
func NewTunnelManager(client *Client) *TunnelManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &TunnelManager{
		client: client,
		ctx:    ctx,
		cancel: cancel,
	}
}

// StartRule starts a single tunnel rule.
func (tm *TunnelManager) StartRule(rule TunnelRule) (*ActiveTunnel, error) {
	rule.Normalize()

	tunnelCtx, tunnelCancel := context.WithCancel(tm.ctx)
	at := &ActiveTunnel{
		Rule:   rule,
		Status: "starting",
		cancel: tunnelCancel,
	}

	tm.mu.Lock()
	tm.tunnels = append(tm.tunnels, at)
	tm.mu.Unlock()

	var err error
	switch rule.Type {
	case "local", "l", "-l":
		err = tm.startLocal(tunnelCtx, at)
	case "remote", "r", "-r":
		err = tm.startRemote(tunnelCtx, at)
	case "dynamic", "socks5", "d", "-d":
		err = tm.startDynamic(tunnelCtx, at)
	default:
		err = fmt.Errorf("unknown tunnel type: %s", rule.Type)
	}

	if err != nil {
		at.Status = "error"
		at.Error = err
		return at, err
	}

	at.Status = "active"
	return at, nil
}

// StartAll starts all given rules.
func (tm *TunnelManager) StartAll(rules []TunnelRule) []error {
	var errs []error
	for _, rule := range rules {
		if _, err := tm.StartRule(rule); err != nil {
			errs = append(errs, fmt.Errorf("tunnel [%s %s->%s]: %w", rule.Type, rule.Local, rule.Remote, err))
		}
	}
	return errs
}

// Close stops all active tunnels.
func (tm *TunnelManager) Close() error {
	tm.cancel()
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for _, at := range tm.tunnels {
		if at.Listener != nil {
			_ = at.Listener.Close()
		}
		at.Status = "closed"
	}
	return nil
}

// GetTunnels returns the snapshot of all tunnels.
func (tm *TunnelManager) GetTunnels() []*ActiveTunnel {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	res := make([]*ActiveTunnel, len(tm.tunnels))
	copy(res, tm.tunnels)
	return res
}

// startLocal implements -L local port forwarding.
func (tm *TunnelManager) startLocal(ctx context.Context, at *ActiveTunnel) error {
	listener, err := net.Listen("tcp", at.Rule.Local)
	if err != nil {
		return fmt.Errorf("listen local %s: %w", at.Rule.Local, err)
	}
	at.Listener = listener

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	go func() {
		for {
			localConn, err := listener.Accept()
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				at.Status = "error"
				at.Error = err
				return
			}

			atomic.AddInt64(&at.ActiveConn, 1)
			go func() {
				defer atomic.AddInt64(&at.ActiveConn, -1)
				defer localConn.Close()

				remoteConn, err := tm.client.Dial("tcp", at.Rule.Remote)
				if err != nil {
					return
				}
				defer remoteConn.Close()

				tm.pipeConns(localConn, remoteConn, at)
			}()
		}
	}()
	return nil
}

// startRemote implements -R remote port forwarding.
func (tm *TunnelManager) startRemote(ctx context.Context, at *ActiveTunnel) error {
	listener, err := tm.client.Listen("tcp", at.Rule.Remote)
	if err != nil {
		return fmt.Errorf("listen remote %s: %w", at.Rule.Remote, err)
	}
	at.Listener = listener

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	go func() {
		for {
			remoteConn, err := listener.Accept()
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				at.Status = "error"
				at.Error = err
				return
			}

			atomic.AddInt64(&at.ActiveConn, 1)
			go func() {
				defer atomic.AddInt64(&at.ActiveConn, -1)
				defer remoteConn.Close()

				localConn, err := net.Dial("tcp", at.Rule.Local)
				if err != nil {
					return
				}
				defer localConn.Close()

				tm.pipeConns(remoteConn, localConn, at)
			}()
		}
	}()
	return nil
}

// startDynamic implements -D dynamic SOCKS5 proxy forwarding.
func (tm *TunnelManager) startDynamic(ctx context.Context, at *ActiveTunnel) error {
	listener, err := net.Listen("tcp", at.Rule.Local)
	if err != nil {
		return fmt.Errorf("listen socks5 %s: %w", at.Rule.Local, err)
	}
	at.Listener = listener

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	go func() {
		for {
			clientConn, err := listener.Accept()
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				at.Status = "error"
				at.Error = err
				return
			}

			atomic.AddInt64(&at.ActiveConn, 1)
			go func() {
				defer atomic.AddInt64(&at.ActiveConn, -1)
				defer clientConn.Close()

				targetConn, err := tm.handleSOCKS5Handshake(clientConn)
				if err != nil {
					return
				}
				defer targetConn.Close()

				tm.pipeConns(clientConn, targetConn, at)
			}()
		}
	}()
	return nil
}

// handleSOCKS5Handshake parses SOCKS5 RFC 1928 and dials the target through SSH.
func (tm *TunnelManager) handleSOCKS5Handshake(clientConn net.Conn) (net.Conn, error) {
	buf := make([]byte, 256)

	// 1. Version identifier and methods negotiation
	if _, err := io.ReadFull(clientConn, buf[:2]); err != nil {
		return nil, err
	}
	if buf[0] != 0x05 {
		return nil, errors.New("unsupported SOCKS version")
	}

	nMethods := int(buf[1])
	if _, err := io.ReadFull(clientConn, buf[:nMethods]); err != nil {
		return nil, err
	}

	// Reply: version 5, no authentication required (0x00)
	if _, err := clientConn.Write([]byte{0x05, 0x00}); err != nil {
		return nil, err
	}

	// 2. Request details
	if _, err := io.ReadFull(clientConn, buf[:4]); err != nil {
		return nil, err
	}

	cmd := buf[1]
	if cmd != 0x01 { // 0x01 = CONNECT
		// Command not supported
		_, _ = clientConn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return nil, fmt.Errorf("unsupported SOCKS5 cmd: %d", cmd)
	}

	var targetHost string
	atyp := buf[3]

	switch atyp {
	case 0x01: // IPv4
		if _, err := io.ReadFull(clientConn, buf[:4]); err != nil {
			return nil, err
		}
		targetHost = net.IP(buf[:4]).String()
	case 0x03: // Domain name
		if _, err := io.ReadFull(clientConn, buf[:1]); err != nil {
			return nil, err
		}
		domainLen := int(buf[0])
		if _, err := io.ReadFull(clientConn, buf[:domainLen]); err != nil {
			return nil, err
		}
		targetHost = string(buf[:domainLen])
	case 0x04: // IPv6
		if _, err := io.ReadFull(clientConn, buf[:16]); err != nil {
			return nil, err
		}
		targetHost = net.IP(buf[:16]).String()
	default:
		_, _ = clientConn.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return nil, fmt.Errorf("unsupported SOCKS5 atyp: %d", atyp)
	}

	// Read Port
	if _, err := io.ReadFull(clientConn, buf[:2]); err != nil {
		return nil, err
	}
	targetPort := binary.BigEndian.Uint16(buf[:2])
	targetAddr := net.JoinHostPort(targetHost, strconv.Itoa(int(targetPort)))

	// Dial target host via SSH Client
	remoteConn, err := tm.client.Dial("tcp", targetAddr)
	if err != nil {
		// General SOCKS server failure (0x01) or Host unreachable (0x04)
		_, _ = clientConn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return nil, fmt.Errorf("ssh dial target %s: %w", targetAddr, err)
	}

	// Reply Success (0x00)
	// BND.ADDR 0.0.0.0:0
	reply := []byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}
	if _, err := clientConn.Write(reply); err != nil {
		remoteConn.Close()
		return nil, err
	}

	return remoteConn, nil
}

// pipeConns handles bidirectional streaming and tracks byte counters.
func (tm *TunnelManager) pipeConns(local, remote net.Conn, at *ActiveTunnel) {
	var wg sync.WaitGroup
	wg.Add(2)

	// local -> remote
	go func() {
		defer wg.Done()
		defer remote.Close()
		n, _ := io.Copy(remote, local)
		atomic.AddInt64(&at.BytesIn, n)
	}()

	// remote -> local
	go func() {
		defer wg.Done()
		defer local.Close()
		n, _ := io.Copy(local, remote)
		atomic.AddInt64(&at.BytesOut, n)
	}()

	wg.Wait()
}
