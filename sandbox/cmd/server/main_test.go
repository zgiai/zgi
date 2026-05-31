package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/config"
)

func TestServeShutsDownWhenContextCanceled(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("expected listener, got %v", err)
	}

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("ok"))
		}),
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- serve(parent, server, listener, 50*time.Millisecond, log.New(io.Discard, "", 0))
	}()

	baseURL := "http://" + listener.Addr().String()
	resp, err := http.Get(baseURL)
	if err != nil {
		t.Fatalf("expected health probe to reach test server, got %v", err)
	}
	_ = resp.Body.Close()

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("expected graceful shutdown, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("expected server to stop after context cancellation")
	}
}

func TestServeDrainsInFlightRequestOnShutdown(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("expected listener, got %v", err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			close(started)
			<-release
			_, _ = w.Write([]byte("drained"))
		}),
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- serve(parent, server, listener, time.Second, log.New(io.Discard, "", 0))
	}()

	respCh := make(chan string, 1)
	go func() {
		resp, err := http.Get("http://" + listener.Addr().String())
		if err != nil {
			respCh <- "request failed: " + err.Error()
			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			respCh <- "read failed: " + err.Error()
			return
		}
		respCh <- string(body)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("expected in-flight request to start")
	}

	cancel()
	select {
	case err := <-errCh:
		t.Fatalf("server returned before in-flight request drained: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)

	select {
	case body := <-respCh:
		if body != "drained" {
			t.Fatalf("expected drained response, got %q", body)
		}
	case <-time.After(time.Second):
		t.Fatal("expected in-flight request to complete")
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("expected graceful shutdown after drain, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("expected server to stop after in-flight request drained")
	}
}

func TestServeReturnsListenError(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("expected listener, got %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("expected closed listener, got %v", err)
	}

	server := &http.Server{Handler: http.NewServeMux()}
	err = serve(parent, server, listener, 50*time.Millisecond, log.New(io.Discard, "", 0))
	if err == nil {
		t.Fatal("expected serve error")
	}
	if !errors.Is(err, net.ErrClosed) {
		t.Fatalf("expected closed listener error, got %v", err)
	}
}

func TestLogStartupConfigOmitsSecrets(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	cfg := config.Config{
		Port:           "2660",
		APIKey:         "secret-api-key",
		RedisPassword:  "secret-redis-password",
		DatabaseURL:    "postgres://user:secret-db-password@127.0.0.1:5432/postgres",
		RedisAddr:      "127.0.0.1:6379",
		WorkerID:       "worker-a",
		RuntimeBackend: "preview",
	}

	logStartupConfig(logger, cfg)

	output := buf.String()
	if !strings.Contains(output, "zgi-sandbox effective config") {
		t.Fatalf("expected effective config log, got %q", output)
	}
	for _, secret := range []string{"secret-api-key", "secret-redis-password", "secret-db-password"} {
		if strings.Contains(output, secret) {
			t.Fatalf("expected secret %q to be omitted from log %q", secret, output)
		}
	}
	if !strings.Contains(output, `"database_configured":true`) {
		t.Fatalf("expected database configured flag, got %q", output)
	}
	if !strings.Contains(output, `"redis_configured":true`) {
		t.Fatalf("expected redis configured flag, got %q", output)
	}
}
