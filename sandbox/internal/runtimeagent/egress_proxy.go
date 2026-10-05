package runtimeagent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/egressproxy"
)

const (
	defaultEgressProxyAddress = "127.0.0.1:3128"
	defaultEgressSocketPath   = "/run/zgi-agent/egress.sock"
)

type egressProxy struct {
	proxyListener  net.Listener
	bridgeListener net.Listener
	server         *http.Server
	available      chan net.Conn
	done           chan struct{}
	closeOnce      sync.Once
	wg             sync.WaitGroup
}

func newEgressProxy(proxyAddress, socketPath string) (*egressProxy, error) {
	proxyAddress = strings.TrimSpace(proxyAddress)
	socketPath = filepath.Clean(strings.TrimSpace(socketPath))
	if proxyAddress == "" || socketPath == "" || !filepath.IsAbs(socketPath) || socketPath == string(filepath.Separator) {
		return nil, errors.New("controlled egress proxy requires absolute addresses")
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o750); err != nil {
		return nil, fmt.Errorf("prepare egress socket directory: %w", err)
	}
	if info, err := os.Lstat(socketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, errors.New("egress socket path exists and is not a socket")
		}
		if err := os.Remove(socketPath); err != nil {
			return nil, fmt.Errorf("remove stale egress socket: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	proxyListener, err := net.Listen("tcp", proxyAddress)
	if err != nil {
		return nil, fmt.Errorf("listen egress proxy: %w", err)
	}
	bridgeListener, err := net.Listen("unix", socketPath)
	if err != nil {
		_ = proxyListener.Close()
		return nil, fmt.Errorf("listen egress bridge: %w", err)
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		_ = proxyListener.Close()
		_ = bridgeListener.Close()
		_ = os.Remove(socketPath)
		return nil, err
	}
	proxy := &egressProxy{
		proxyListener: proxyListener, bridgeListener: bridgeListener,
		available: make(chan net.Conn, 8), done: make(chan struct{}),
	}
	proxy.server = &http.Server{Handler: http.HandlerFunc(proxy.handle), ReadHeaderTimeout: 5 * time.Second}
	proxy.wg.Add(2)
	go proxy.serveProxy()
	go proxy.acceptBridges()
	return proxy, nil
}

func (p *egressProxy) serveProxy() {
	defer p.wg.Done()
	if err := p.server.Serve(p.proxyListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		// The Runtime Agent control server remains authoritative; proxy errors
		// are surfaced to callers as connection failures rather than panics.
	}
}

func (p *egressProxy) acceptBridges() {
	defer p.wg.Done()
	for {
		connection, err := p.bridgeListener.Accept()
		if err != nil {
			select {
			case <-p.done:
				return
			default:
			}
			continue
		}
		select {
		case p.available <- connection:
		case <-p.done:
			_ = connection.Close()
		}
	}
}

func (p *egressProxy) takeBridge(ctx context.Context) (net.Conn, error) {
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	select {
	case connection := <-p.available:
		if connection == nil {
			return nil, errors.New("egress bridge is closed")
		}
		return connection, nil
	case <-timer.C:
		return nil, errors.New("egress bridge is not connected")
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.done:
		return nil, errors.New("egress bridge is closed")
	}
}

func (p *egressProxy) handle(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodConnect {
		http.Error(writer, "only HTTPS CONNECT is supported", http.StatusForbidden)
		return
	}
	bridge, err := p.takeBridge(request.Context())
	if err != nil {
		http.Error(writer, "controlled egress is unavailable", http.StatusBadGateway)
		return
	}
	if err := egressproxy.WriteFrame(bridge, egressproxy.Frame{Type: egressproxy.FrameTypeConnect, Target: request.Host}); err != nil {
		_ = bridge.Close()
		http.Error(writer, "controlled egress request failed", http.StatusBadGateway)
		return
	}
	result, err := egressproxy.ReadFrame(bridge)
	if err != nil || result.Type != egressproxy.FrameTypeResult || !result.OK {
		_ = bridge.Close()
		http.Error(writer, "controlled egress destination denied", http.StatusForbidden)
		return
	}
	hijacker, ok := writer.(http.Hijacker)
	if !ok {
		_ = bridge.Close()
		http.Error(writer, "proxy does not support connection hijacking", http.StatusInternalServerError)
		return
	}
	client, buffered, err := hijacker.Hijack()
	if err != nil {
		_ = bridge.Close()
		return
	}
	if _, err := buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		_ = client.Close()
		_ = bridge.Close()
		return
	}
	if err := buffered.Flush(); err != nil {
		_ = client.Close()
		_ = bridge.Close()
		return
	}
	_ = tunnel(client, bridge)
}

func tunnel(client, bridge net.Conn) error {
	result := make(chan error, 2)
	go func() { _, err := ioCopy(client, bridge); result <- err }()
	go func() { _, err := ioCopy(bridge, client); result <- err }()
	err := <-result
	_ = client.Close()
	_ = bridge.Close()
	return err
}

func ioCopy(destination, source net.Conn) (int64, error) {
	return copyLimit(destination, source, egressproxy.MaxTunnelBytes)
}

func copyLimit(destination net.Conn, source net.Conn, limit int64) (int64, error) {
	return io.CopyN(destination, source, limit)
}

func (p *egressProxy) Close() error {
	var result error
	p.closeOnce.Do(func() {
		close(p.done)
		result = errors.Join(p.server.Close(), p.proxyListener.Close(), p.bridgeListener.Close())
		for {
			select {
			case connection := <-p.available:
				_ = connection.Close()
			default:
				_ = os.Remove(p.bridgeListener.Addr().String())
				p.wg.Wait()
				return
			}
		}
	})
	return result
}
