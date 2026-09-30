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
	e2bprovider "github.com/zgiai/zgi-sandbox/internal/hostedruntime/e2b"
	"github.com/zgiai/zgi-sandbox/internal/localruntime"
	containerdprovider "github.com/zgiai/zgi-sandbox/internal/localruntime/containerd"
	"github.com/zgiai/zgi-sandbox/internal/storage"
	"github.com/zgiai/zgi-sandbox/internal/workspacecheckpoint"
)

const (
	localBackendName = "local-gvisor"
	e2bBackendName   = "e2b"
)

type runtimeBackend interface {
	agentexecution.Backend
	agentexecution.ExecutionBackend
}

type workerConfig struct {
	ListenAddress     string
	APIKey            string
	DatabaseURL       string
	WorkerID          string
	RootDir           string
	CheckpointDir     string
	CheckpointS3      workspacecheckpoint.S3Config
	ContainerdAddress string
	Profiles          map[string]containerdprovider.Profile
	MaxWorkers        int
	IdleTTL           time.Duration
	ShutdownTimeout   time.Duration
	StartupTimeout    time.Duration
	MemoryBytes       int64
	PIDs              int64
	CPUPeriod         uint64
	CPUQuota          int64
	Backend           string
	E2BAPIURL         string
	E2BDomain         string
	E2BAPIKey         string
	E2BProfiles       map[string]e2bprovider.Profile
	E2BAgentPort      uint16
	E2BEnvdPort       uint16
	E2BTimeoutSeconds int
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
	backendName, backend, closeBackend, err := newRuntimeBackend(cfg)
	if err != nil {
		return err
	}
	defer closeBackend()
	controlBackends := map[string]agentexecution.Backend{backendName: backend}
	executionBackends := map[string]agentexecution.ExecutionBackend{backendName: backend}
	var checkpoints agentexecution.WorkspaceCheckpointStore
	if cfg.CheckpointS3.Bucket != "" {
		checkpoints, err = workspacecheckpoint.NewS3Store(parent, cfg.CheckpointS3)
		if err != nil {
			return err
		}
	} else if cfg.CheckpointDir != "" {
		fileStore, err := workspacecheckpoint.NewFileStore(cfg.CheckpointDir)
		if err != nil {
			return err
		}
		defer fileStore.Close()
		checkpoints = fileStore
	}
	control, err := agentexecution.NewService(store, controlBackends, agentexecution.ServiceOptions{
		IdleTTL: cfg.IdleTTL, Checkpoints: checkpoints, Jobs: store,
	})
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
	if checkpoints != nil {
		retentionReaper, err := agentexecution.NewRetentionReaper(store, control, nil)
		if err != nil {
			stopWorkers()
			workers.Wait()
			return err
		}
		startLoop("retention-reaper", agentexecution.OneShotFunc(retentionReaper.RunOne), time.Second)
	}

	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	serverResult := make(chan error, 1)
	go func() {
		logger.Printf("zgi Agent Runtime worker %s listening on %s with backend %s", cfg.WorkerID, listener.Addr(), backendName)
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
	hostname, _ := os.Hostname()
	fallbackWorkerID := fmt.Sprintf("%s-%d", hostname, os.Getpid())
	cfg := workerConfig{
		ListenAddress: envString("ZGI_AGENT_RUNTIME_LISTEN", "127.0.0.1:2670"),
		APIKey:        strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_API_KEY")),
		DatabaseURL:   strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_DATABASE_URL")),
		WorkerID:      envString("ZGI_AGENT_RUNTIME_WORKER_ID", fallbackWorkerID),
		RootDir:       envString("ZGI_AGENT_RUNTIME_ROOT", "/var/lib/zgi-agent-runtime"),
		CheckpointDir: strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_CHECKPOINT_DIR")),
		CheckpointS3: workspacecheckpoint.S3Config{
			Bucket:       strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_CHECKPOINT_S3_BUCKET")),
			Region:       strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_CHECKPOINT_S3_REGION")),
			Endpoint:     strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_CHECKPOINT_S3_ENDPOINT")),
			Prefix:       strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_CHECKPOINT_S3_PREFIX")),
			UsePathStyle: strings.EqualFold(strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_CHECKPOINT_S3_PATH_STYLE")), "true"),
		},
		ContainerdAddress: envString("ZGI_AGENT_RUNTIME_CONTAINERD_ADDRESS", "/run/containerd/containerd.sock"),
		Profiles:          map[string]containerdprovider.Profile{},
		MaxWorkers:        envInt("ZGI_AGENT_RUNTIME_MAX_WORKERS", 4),
		IdleTTL:           time.Duration(envInt("ZGI_AGENT_RUNTIME_IDLE_TTL_SECONDS", 1800)) * time.Second,
		ShutdownTimeout:   time.Duration(envInt("ZGI_AGENT_RUNTIME_SHUTDOWN_TIMEOUT_SECONDS", 30)) * time.Second,
		StartupTimeout:    time.Duration(envInt("ZGI_AGENT_RUNTIME_STARTUP_TIMEOUT_SECONDS", 30)) * time.Second,
		MemoryBytes:       envInt64("ZGI_AGENT_RUNTIME_MEMORY_BYTES", 512*1024*1024),
		PIDs:              envInt64("ZGI_AGENT_RUNTIME_PIDS", 128),
		CPUPeriod:         uint64(envInt64("ZGI_AGENT_RUNTIME_CPU_PERIOD", 100000)),
		CPUQuota:          envInt64("ZGI_AGENT_RUNTIME_CPU_QUOTA", 100000),
		Backend:           envString("ZGI_AGENT_RUNTIME_BACKEND", localBackendName),
		E2BAPIURL:         strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_E2B_API_URL")),
		E2BDomain:         strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_E2B_DOMAIN")),
		E2BAPIKey:         strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_E2B_API_KEY")),
		E2BProfiles:       map[string]e2bprovider.Profile{},
		E2BAgentPort:      envUint16("ZGI_AGENT_RUNTIME_E2B_AGENT_PORT", 49984),
		E2BEnvdPort:       envUint16("ZGI_AGENT_RUNTIME_E2B_ENVD_PORT", 49983),
		E2BTimeoutSeconds: envInt("ZGI_AGENT_RUNTIME_E2B_TIMEOUT_SECONDS", 1800),
	}
	if cfg.APIKey == "" || cfg.DatabaseURL == "" {
		return workerConfig{}, errors.New("ZGI_AGENT_RUNTIME_API_KEY and ZGI_AGENT_RUNTIME_DATABASE_URL are required")
	}
	if cfg.MaxWorkers <= 0 || cfg.IdleTTL <= 0 || cfg.ShutdownTimeout <= 0 {
		return workerConfig{}, errors.New("positive Agent Runtime worker and timeout limits are required")
	}
	if cfg.CheckpointDir != "" && cfg.CheckpointS3.Bucket != "" {
		return workerConfig{}, errors.New("configure either a local checkpoint directory or S3 checkpoint bucket, not both")
	}
	if cfg.CheckpointS3.Bucket == "" && (cfg.CheckpointS3.Region != "" || cfg.CheckpointS3.Endpoint != "" || cfg.CheckpointS3.Prefix != "" || cfg.CheckpointS3.UsePathStyle) {
		return workerConfig{}, errors.New("checkpoint S3 bucket is required when S3 options are set")
	}
	switch cfg.Backend {
	case localBackendName:
		if err := decodeProfiles("ZGI_AGENT_RUNTIME_PROFILES_JSON", &cfg.Profiles); err != nil {
			return workerConfig{}, err
		}
		if len(cfg.Profiles) == 0 || cfg.StartupTimeout <= 0 || cfg.MemoryBytes <= 0 || cfg.PIDs <= 0 || cfg.CPUPeriod == 0 || cfg.CPUQuota <= 0 {
			return workerConfig{}, errors.New("local Agent Runtime profiles and positive startup and resource limits are required")
		}
	case e2bBackendName:
		if err := decodeProfiles("ZGI_AGENT_RUNTIME_E2B_PROFILES_JSON", &cfg.E2BProfiles); err != nil {
			return workerConfig{}, err
		}
		if cfg.E2BAPIKey == "" || len(cfg.E2BProfiles) == 0 || cfg.E2BAgentPort == 0 || cfg.E2BEnvdPort == 0 || cfg.E2BTimeoutSeconds <= 0 {
			return workerConfig{}, errors.New("E2B API key, approved profiles, ports, and timeout are required")
		}
	default:
		return workerConfig{}, fmt.Errorf("unsupported ZGI_AGENT_RUNTIME_BACKEND %q", cfg.Backend)
	}
	if _, _, err := net.SplitHostPort(cfg.ListenAddress); err != nil {
		return workerConfig{}, fmt.Errorf("parse ZGI_AGENT_RUNTIME_LISTEN: %w", err)
	}
	return cfg, nil
}

func newRuntimeBackend(cfg workerConfig) (string, runtimeBackend, func(), error) {
	switch cfg.Backend {
	case localBackendName:
		provider, err := containerdprovider.New(containerdprovider.Options{
			Address: cfg.ContainerdAddress, RootDir: cfg.RootDir, Profiles: cfg.Profiles,
			StartupLimit: cfg.StartupTimeout,
			MemoryBytes:  cfg.MemoryBytes, PIDs: cfg.PIDs, CPUPeriod: cfg.CPUPeriod, CPUQuota: cfg.CPUQuota,
		})
		if err != nil {
			return "", nil, func() {}, err
		}
		backend, err := localruntime.New(provider)
		if err != nil {
			_ = provider.Close()
			return "", nil, func() {}, err
		}
		return localBackendName, backend, func() { _ = provider.Close() }, nil
	case e2bBackendName:
		backend, err := e2bprovider.New(e2bprovider.Options{
			APIURL: cfg.E2BAPIURL, Domain: cfg.E2BDomain, APIKey: cfg.E2BAPIKey,
			Profiles: cfg.E2BProfiles, AgentPort: cfg.E2BAgentPort, EnvdPort: cfg.E2BEnvdPort,
			TimeoutSeconds: cfg.E2BTimeoutSeconds,
		})
		if err != nil {
			return "", nil, func() {}, err
		}
		return e2bBackendName, backend, func() {}, nil
	default:
		return "", nil, func() {}, fmt.Errorf("unsupported Agent Runtime backend %q", cfg.Backend)
	}
}

func decodeProfiles(name string, target any) error {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		value = "{}"
	}
	if err := json.Unmarshal([]byte(value), target); err != nil {
		return fmt.Errorf("parse %s: %w", name, err)
	}
	return nil
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

func envUint16(name string, fallback uint16) uint16 {
	value, err := strconv.ParseUint(strings.TrimSpace(os.Getenv(name)), 10, 16)
	if err != nil {
		return fallback
	}
	return uint16(value)
}
