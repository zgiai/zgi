package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/app"
	"github.com/zgiai/zgi-sandbox/internal/config"
)

func main() {
	cfg := config.FromEnv()
	if err := run(context.Background(), cfg, log.Default()); err != nil {
		log.Fatal(err)
	}
}

func run(parent context.Context, cfg config.Config, logger *log.Logger) error {
	server, err := app.NewServer(cfg)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		return err
	}

	httpServer := &http.Server{
		Handler: server.Handler(),
	}
	return serve(parent, httpServer, listener, time.Duration(cfg.ShutdownTimeoutSeconds)*time.Second, logger)
}

func serve(parent context.Context, server *http.Server, listener net.Listener, shutdownTimeout time.Duration, logger *log.Logger) error {
	if shutdownTimeout <= 0 {
		shutdownTimeout = 10 * time.Second
	}
	if logger == nil {
		logger = log.Default()
	}

	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Printf("zgi-sandbox listening on http://%s", listener.Addr().String())
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		stop()
		logger.Printf("zgi-sandbox shutting down with timeout %s", shutdownTimeout)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		select {
		case err := <-errCh:
			return err
		case <-shutdownCtx.Done():
			return shutdownCtx.Err()
		}
	}
}
