//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/config"
	"github.com/zgiai/zgi-sandbox/internal/localruntime"
	containerdprovider "github.com/zgiai/zgi-sandbox/internal/localruntime/containerd"
	"github.com/zgiai/zgi-sandbox/internal/storage"
)

const localBackendName = "local-gvisor"

type workerConfig struct {
	ListenAddress     string
	APIKey            string
	DatabaseURL       string
	WorkerID          string
	RootDir           string
	ContainerdAddress string
	Profiles          map[string]containerdprovider.Profile
	MaxWorkers        int
	IdleTTL           time.Duration
	ShutdownTimeout   time.Duration
	MemoryBytes       int64
	PIDs              int64
	CPUPeriod         uint64
	CPUQuota          int64
}

func run(parent context.Context, logger *log.Logger) error {
	if logger == nil {
		logger = log.Default()
	}
	cfg, err := workerConfigFromEnv()
	if err != nil {
		return err
	}
	store, err := storage.Open(config.Config{
		DatabaseURL: cfg.DatabaseURL, MaxWorkers: cfg.MaxWorkers,
		ObserverRetentionDays: 7, ObserverMaxEvents: 10000,
	})
	if err != nil {
		return err
	}
	defer store.Close()
	provider, err := containerdprovider.New(containerdprovider.Options{
		Address: cfg.ContainerdAddress, RootDir: cfg.RootDir, Profiles: cfg.Profiles,
		MemoryBytes: cfg.MemoryBytes, PIDs: cfg.PIDs, CPUPeriod: cfg.CPUPeriod, CPUQuota: cfg.CPUQuota,
	})
	if err != nil {
		return err
	}
	defer provider.Close()
	backend, err := localruntime.New(provider)
	if err != nil {
		return err
	}
	controlBackends := map[string]agentexecution.Backend{localBackendName: backend}
	executionBackends := map[string]agentexecution.ExecutionBackend{localBackendName: backend}
	control, err := agentexecution.NewService(store, controlBackends, agentexecution.ServiceOptions{IdleTTL: cfg.IdleTTL})
	if err != nil {
		return err
	}
	data, err := agentexecution.NewDataPlaneService(store, store, executionBackends, agentexecution.DataPlaneOptions{IdleTTL: cfg.IdleTTL})
	if err != nil {
		return err
	}
	api, err := agentexecution.NewHTTPAPI(control, data, agentexecution.HTTPAPIOptions{APIKey: cfg.APIKey})
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	if err := api.Register(mux); err != nil {
		return err
	}
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
		defer cancel()
		if err := store.Ping(ctx); err != nil {
			http.Error(w, "unhealthy", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	listener, err := net.Listen("tcp", cfg.ListenAddress)
	if err != nil {
		return err
	}
	defer listener.Close()
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	workerContext, stopWorkers := context.WithCancel(ctx)
	var workers sync.WaitGroup
	startLoop := func(name string, worker agentexecution.OneShotWorker, idle time.Duration) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			err := agentexecution.RunLoop(workerContext, worker, agentexecution.LoopOptions{
				IdleInterval: idle, ErrorInterval: time.Second,
				OnError: func(err error) { logger.Printf("Agent Runtime %s error: %v", name, err) },
			})
			if err != nil && !errors.Is(err, context.Canceled) {
				logger.Printf("Agent Runtime %s stopped: %v", name, err)
			}
		}()
	}
	for index := 0; index < cfg.MaxWorkers; index++ {
		jobWorker, err := agentexecution.NewWorker(store, executionBackends, agentexecution.WorkerOptions{
			OwnerID: fmt.Sprintf("%s-job-%d", cfg.WorkerID, index+1),
		})
		if err != nil {
			stopWorkers()
			workers.Wait()
			return err
		}
		startLoop(fmt.Sprintf("job-%d", index+1), jobWorker, 100*time.Millisecond)
	}
	reconciler, err := agentexecution.NewReconciler(store, controlBackends, agentexecution.ReconcilerOptions{OwnerID: cfg.WorkerID + "-cleanup"})
	if err != nil {
		stopWorkers()
		workers.Wait()
		return err
	}
	startLoop("cleanup", agentexecution.OneShotFunc(reconciler.ReconcileOne), time.Second)
	reaper, err := agentexecution.NewIdleReaper(store, control, agentexecution.IdleReaperOptions{})
	if err != nil {
		stopWorkers()
		workers.Wait()
		return err
	}
	startLoop("idle-reaper", reaper, time.Second)

	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	serverResult := make(chan error, 1)
	go func() {
		logger.Printf("zgi Agent Runtime worker %s listening on %s with backend %s", cfg.WorkerID, listener.Addr(), localBackendName)
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serverResult <- err
	}()

	select {
	case err := <-serverResult:
		stopWorkers()
		workers.Wait()
		return err
	case <-ctx.Done():
		stopWorkers()
		shutdownContext, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		shutdownErr := server.Shutdown(shutdownContext)
		workers.Wait()
		serveErr := <-serverResult
		return errors.Join(shutdownErr, serveErr)
	}
}

func workerConfigFromEnv() (workerConfig, error) {
	profiles := map[string]containerdprovider.Profile{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_PROFILES_JSON"))), &profiles); err != nil {
		return workerConfig{}, fmt.Errorf("parse ZGI_AGENT_RUNTIME_PROFILES_JSON: %w", err)
	}
	hostname, _ := os.Hostname()
	fallbackWorkerID := fmt.Sprintf("%s-%d", hostname, os.Getpid())
	cfg := workerConfig{
		ListenAddress:     envString("ZGI_AGENT_RUNTIME_LISTEN", "127.0.0.1:2670"),
		APIKey:            strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_API_KEY")),
		DatabaseURL:       strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_DATABASE_URL")),
		WorkerID:          envString("ZGI_AGENT_RUNTIME_WORKER_ID", fallbackWorkerID),
		RootDir:           envString("ZGI_AGENT_RUNTIME_ROOT", "/var/lib/zgi-agent-runtime"),
		ContainerdAddress: envString("ZGI_AGENT_RUNTIME_CONTAINERD_ADDRESS", "/run/containerd/containerd.sock"),
		Profiles:          profiles,
		MaxWorkers:        envInt("ZGI_AGENT_RUNTIME_MAX_WORKERS", 4),
		IdleTTL:           time.Duration(envInt("ZGI_AGENT_RUNTIME_IDLE_TTL_SECONDS", 1800)) * time.Second,
		ShutdownTimeout:   time.Duration(envInt("ZGI_AGENT_RUNTIME_SHUTDOWN_TIMEOUT_SECONDS", 30)) * time.Second,
		MemoryBytes:       envInt64("ZGI_AGENT_RUNTIME_MEMORY_BYTES", 512*1024*1024),
		PIDs:              envInt64("ZGI_AGENT_RUNTIME_PIDS", 128),
		CPUPeriod:         uint64(envInt64("ZGI_AGENT_RUNTIME_CPU_PERIOD", 100000)),
		CPUQuota:          envInt64("ZGI_AGENT_RUNTIME_CPU_QUOTA", 100000),
	}
	if cfg.APIKey == "" || cfg.DatabaseURL == "" {
		return workerConfig{}, errors.New("ZGI_AGENT_RUNTIME_API_KEY and ZGI_AGENT_RUNTIME_DATABASE_URL are required")
	}
	if len(cfg.Profiles) == 0 || cfg.MaxWorkers <= 0 || cfg.IdleTTL <= 0 || cfg.ShutdownTimeout <= 0 || cfg.MemoryBytes <= 0 || cfg.PIDs <= 0 || cfg.CPUPeriod == 0 || cfg.CPUQuota <= 0 {
		return workerConfig{}, errors.New("Agent Runtime profiles and positive resource, worker, and timeout limits are required")
	}
	if _, _, err := net.SplitHostPort(cfg.ListenAddress); err != nil {
		return workerConfig{}, fmt.Errorf("parse ZGI_AGENT_RUNTIME_LISTEN: %w", err)
	}
	return cfg, nil
}

func envString(name string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil {
		return fallback
	}
	return value
}

func envInt64(name string, fallback int64) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(name)), 10, 64)
	if err != nil {
		return fallback
	}
	return value
}
