package main

import (
	"context"
	"encoding/json"
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

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
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
	token, err := runtimeAgentToken()
	if err != nil {
		return err
	}
	workspace := envValue("ZGI_RUNTIME_AGENT_WORKSPACE", "/workspace")
	var egressAllowedHosts []string
	if raw := strings.TrimSpace(os.Getenv("ZGI_RUNTIME_AGENT_EGRESS_ALLOWED_HOSTS_JSON")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &egressAllowedHosts); err != nil || len(egressAllowedHosts) == 0 {
			return errors.New("ZGI_RUNTIME_AGENT_EGRESS_ALLOWED_HOSTS_JSON must be a non-empty hostname array")
		}
	}
	backend, err := runtimeagent.New(runtimeagent.Options{
		WorkspaceRoot:             workspace,
		NetworkMode:               envValue("ZGI_RUNTIME_AGENT_NETWORK_MODE", "disabled"),
		EgressMode:                envValue("ZGI_RUNTIME_AGENT_EGRESS_MODE", runtimeagent.EgressModeBridge),
		EgressProxyAddress:        envValue("ZGI_RUNTIME_AGENT_EGRESS_PROXY_ADDRESS", "127.0.0.1:3128"),
		EgressAllowedHosts:        egressAllowedHosts,
		EgressSocketPath:          envValue("ZGI_RUNTIME_AGENT_EGRESS_SOCKET", "/run/zgi-agent/egress.sock"),
		DependencyPreparation:     envBool("ZGI_RUNTIME_AGENT_DEPENDENCY_PREPARATION_ENABLED", false),
		DependencyPreparationMode: envValue("ZGI_RUNTIME_AGENT_DEPENDENCY_PREPARATION_MODE", ""),
		DependencyInstallation:    envBool("ZGI_RUNTIME_AGENT_DEPENDENCY_INSTALLATION_ENABLED", false),
		PackageNetworkMode:        envValue("ZGI_RUNTIME_AGENT_PACKAGE_NETWORK_MODE", ""),
		JobUID:                    uint32(envInt64("ZGI_RUNTIME_AGENT_JOB_UID", 0)),
		JobGID:                    uint32(envInt64("ZGI_RUNTIME_AGENT_JOB_GID", 0)),
		MaxUploadBytes:            envInt64("ZGI_RUNTIME_AGENT_MAX_UPLOAD_BYTES", 64*1024*1024),
		MaxDownloadBytes:          envInt64("ZGI_RUNTIME_AGENT_MAX_DOWNLOAD_BYTES", 64*1024*1024),
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
	var packageListener net.Listener
	var packageCleanup func()
	capabilities, capabilityErr := backend.Capabilities(context.Background(), agentexecution.Allocation{})
	if capabilityErr != nil {
		return capabilityErr
	}
	if capabilities.DependencyInstallation {
		packageListener, packageCleanup, err = listenRuntimePackage(envValue("ZGI_RUNTIME_AGENT_PACKAGE_LISTEN", "unix:///run/zgi-agent/package.sock"))
		if err != nil {
			return err
		}
		defer packageCleanup()
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	var packageServer *http.Server
	if packageListener != nil {
		packageServer = &http.Server{Handler: http.HandlerFunc(handler.ServePackageHTTP), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
		go func() {
			if serveErr := packageServer.Serve(packageListener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
				log.Printf("package install server stopped: %v", serveErr)
			}
		}()
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
			if packageServer != nil {
				_ = packageServer.Shutdown(shutdownContext)
			}
			return err
		}
		if packageServer != nil {
			_ = packageServer.Shutdown(shutdownContext)
		}
		return <-serveResult
	}
}

func envBool(name string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func runtimeAgentToken() (string, error) {
	tokenFile := strings.TrimSpace(os.Getenv("ZGI_RUNTIME_AGENT_TOKEN_FILE"))
	if tokenFile == "" {
		token := strings.TrimSpace(os.Getenv("ZGI_RUNTIME_AGENT_TOKEN"))
		if token == "" {
			return "", errors.New("ZGI_RUNTIME_AGENT_TOKEN or ZGI_RUNTIME_AGENT_TOKEN_FILE is required")
		}
		return token, nil
	}
	tokenFile = filepath.Clean(tokenFile)
	if !filepath.IsAbs(tokenFile) || tokenFile == string(filepath.Separator) {
		return "", errors.New("ZGI_RUNTIME_AGENT_TOKEN_FILE must be an absolute non-root path")
	}
	info, err := os.Lstat(tokenFile)
	if err != nil {
		return "", fmt.Errorf("inspect ZGI_RUNTIME_AGENT_TOKEN_FILE: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("ZGI_RUNTIME_AGENT_TOKEN_FILE must be a regular file")
	}
	if err := os.Chmod(tokenFile, 0o600); err != nil {
		return "", fmt.Errorf("restrict ZGI_RUNTIME_AGENT_TOKEN_FILE: %w", err)
	}
	value, err := os.ReadFile(tokenFile)
	if err != nil {
		return "", fmt.Errorf("read ZGI_RUNTIME_AGENT_TOKEN_FILE: %w", err)
	}
	if err := os.Remove(tokenFile); err != nil {
		return "", fmt.Errorf("remove ZGI_RUNTIME_AGENT_TOKEN_FILE: %w", err)
	}
	token := strings.TrimSpace(string(value))
	if token == "" {
		return "", errors.New("ZGI_RUNTIME_AGENT_TOKEN_FILE is empty")
	}
	return token, nil
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

func listenRuntimePackage(value string) (net.Listener, func(), error) {
	listener, cleanup, err := listenRuntimeAgent(value)
	if err != nil {
		return nil, func() {}, err
	}
	parsed, parseErr := url.Parse(strings.TrimSpace(value))
	if parseErr != nil || parsed.Scheme != "unix" {
		cleanup()
		return nil, func() {}, errors.New("package install listener must use unix://")
	}
	if err := os.Chmod(filepath.Clean(parsed.Path), 0o660); err != nil {
		cleanup()
		return nil, func() {}, err
	}
	if err := os.Chown(filepath.Clean(parsed.Path), 0, 65532); err != nil {
		cleanup()
		return nil, func() {}, err
	}
	return listener, cleanup, nil
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
