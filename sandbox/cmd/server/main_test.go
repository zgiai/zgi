package main

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"testing"
	"time"
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
