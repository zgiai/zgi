package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/runtimeagent"
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func run(parent context.Context) error {
	if runtime.GOOS != "linux" {
		return errors.New("zgi runtime agent requires Linux")
	}
	token := strings.TrimSpace(os.Getenv("ZGI_RUNTIME_AGENT_TOKEN"))
	if token == "" {
		return errors.New("ZGI_RUNTIME_AGENT_TOKEN is required")
	}
	workspace := envValue("ZGI_RUNTIME_AGENT_WORKSPACE", "/workspace")
	backend, err := runtimeagent.New(runtimeagent.Options{
		WorkspaceRoot:    workspace,
		NetworkMode:      envValue("ZGI_RUNTIME_AGENT_NETWORK_MODE", "disabled"),
		MaxUploadBytes:   envInt64("ZGI_RUNTIME_AGENT_MAX_UPLOAD_BYTES", 64*1024*1024),
		MaxDownloadBytes: envInt64("ZGI_RUNTIME_AGENT_MAX_DOWNLOAD_BYTES", 64*1024*1024),
	})
	if err != nil {
		return err
	}
	defer backend.Close()
	handler, err := runtimeagent.NewHTTPServer(backend, runtimeagent.HTTPServerOptions{
		Token:                  token,
		MaxRequestBytes:        envInt64("ZGI_RUNTIME_AGENT_MAX_REQUEST_BYTES", 1024*1024),
		MaxBufferedOutputBytes: envInt64("ZGI_RUNTIME_AGENT_MAX_BUFFERED_OUTPUT_BYTES", 8*1024*1024),
	})
	if err != nil {
		return err
	}
	defer handler.Close()

	listener, cleanup, err := listenRuntimeAgent(envValue("ZGI_RUNTIME_AGENT_LISTEN", "unix:///run/zgi-agent/runtime.sock"))
	if err != nil {
		return err
	}
	defer cleanup()
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	serveResult := make(chan error, 1)
	go func() {
		log.Printf("zgi runtime agent listening on %s", listener.Addr())
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serveResult <- err
	}()
	select {
	case err := <-serveResult:
		return err
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			return err
		}
		return <-serveResult
	}
}

func listenRuntimeAgent(value string) (net.Listener, func(), error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return nil, func() {}, fmt.Errorf("parse ZGI_RUNTIME_AGENT_LISTEN: %w", err)
	}
	switch parsed.Scheme {
	case "unix":
		socketPath := filepath.Clean(parsed.Path)
		if !filepath.IsAbs(socketPath) || socketPath == string(filepath.Separator) {
			return nil, func() {}, errors.New("runtime agent Unix socket path must be an absolute non-root path")
		}
		if err := os.MkdirAll(filepath.Dir(socketPath), 0o750); err != nil {
			return nil, func() {}, err
		}
		if info, err := os.Lstat(socketPath); err == nil {
			if info.Mode()&os.ModeSocket == 0 {
				return nil, func() {}, errors.New("runtime agent listen path exists and is not a socket")
			}
			if err := os.Remove(socketPath); err != nil {
				return nil, func() {}, err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, func() {}, err
		}
		listener, err := net.Listen("unix", socketPath)
		if err != nil {
			return nil, func() {}, err
		}
		if err := os.Chmod(socketPath, 0o600); err != nil {
			_ = listener.Close()
			_ = os.Remove(socketPath)
			return nil, func() {}, err
		}
		return listener, func() {
			_ = listener.Close()
			_ = os.Remove(socketPath)
		}, nil
	case "tcp":
		if parsed.Host == "" {
			return nil, func() {}, errors.New("runtime agent TCP address is required")
		}
		listener, err := net.Listen("tcp", parsed.Host)
		if err != nil {
			return nil, func() {}, err
		}
		return listener, func() { _ = listener.Close() }, nil
	default:
		return nil, func() {}, errors.New("ZGI_RUNTIME_AGENT_LISTEN must use unix:// or tcp://")
	}
}

func envValue(name string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envInt64(name string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
