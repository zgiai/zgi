package bootstrap_test

import (
	"context"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/bootstrap/fxapp"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
)

func TestValidateAppModule(t *testing.T) {
	t.Parallel()

	err := fx.ValidateApp(
		fxapp.Module,
		fx.Logger(fxtest.NewTestPrinter(t)),
	)
	if err != nil {
		t.Fatalf("fx.ValidateApp(fxapp.Module) = %v, want nil", err)
	}
}

func TestValidateAppModuleWithGRPCDisabled(t *testing.T) {
	previous := config.GlobalConfig
	t.Cleanup(func() {
		config.GlobalConfig = previous
	})

	cfg, err := config.LoadFromFile(writeBootstrapEnvFile(t, map[string]string{
		"SERVER_MODE":                  "release",
		"ENV":                          "production",
		"SECRET_KEY":                   "test-secret",
		"API_KEY_ENCRYPTION_KEY":       "0123456789abcdef0123456789abcdef",
		"EMAIL_MAIL_DEFAULT_SEND_FROM": "noreply@example.com",
		"EMAIL_RESEND_API_KEY":         "test-api-key",
		"GRPC_ENABLED":                 "false",
		"GRPC_PORT":                    "invalid-port",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}
	config.GlobalConfig = cfg

	err = fx.ValidateApp(
		fxapp.Module,
		fx.Logger(fxtest.NewTestPrinter(t)),
	)
	if err != nil {
		t.Fatalf("fx.ValidateApp(fxapp.Module) with gRPC disabled = %v, want nil", err)
	}
}

func writeBootstrapEnvFile(t *testing.T, values map[string]string) string {
	t.Helper()

	path := t.TempDir() + "/.env"
	var content string
	for key, value := range values {
		content += key + "=" + value + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}
	return path
}

func TestRegisterGRPCServerLifecycle(t *testing.T) {
	t.Parallel()

	lc := &recordingLifecycle{}
	server := newFakeGRPCServer()
	listener := newFakeListener("127.0.0.1:50051")
	log := newTestLogger(t)

	fxapp.RegisterGRPCServerLifecycle(lc, server, listener, log)

	if got := len(lc.hooks); got != 1 {
		t.Fatalf("hooks len = %d, want 1", got)
	}

	if err := lc.hooks[0].OnStart(context.Background()); err != nil {
		t.Fatalf("OnStart() error = %v, want nil", err)
	}

	server.waitStarted(t)
	if got := server.startedListener(); got != listener {
		t.Fatalf("Serve listener = %v, want %v", got, listener)
	}

	if err := lc.hooks[0].OnStop(context.Background()); err != nil {
		t.Fatalf("OnStop() error = %v, want nil", err)
	}

	if got := server.stopCount(); got != 1 {
		t.Fatalf("Stop calls = %d, want 1", got)
	}
	if got := listener.closeCount(); got != 1 {
		t.Fatalf("listener close calls = %d, want 1", got)
	}
}

func TestRegisterGRPCServerLifecycleDisabled(t *testing.T) {
	t.Parallel()

	lc := &recordingLifecycle{}
	server := newFakeGRPCServer()
	log := newTestLogger(t)

	fxapp.RegisterGRPCServerLifecycle(lc, server, nil, log)

	if got := len(lc.hooks); got != 0 {
		t.Fatalf("hooks len = %d, want 0", got)
	}
	if got := server.startCount(); got != 0 {
		t.Fatalf("Serve calls = %d, want 0", got)
	}
	if got := server.stopCount(); got != 0 {
		t.Fatalf("Stop calls = %d, want 0", got)
	}
}

func TestRegisterTaskManagerLifecycle(t *testing.T) {
	t.Parallel()

	lc := &recordingLifecycle{}
	manager := newFakeTaskManager()
	registry := newFakeTaskHandlerRegistrar()
	log := newTestLogger(t)

	fxapp.RegisterTaskManagerLifecycle(lc, manager, registry, log)

	if got := len(lc.hooks); got != 1 {
		t.Fatalf("hooks len = %d, want 1", got)
	}

	if err := lc.hooks[0].OnStart(context.Background()); err != nil {
		t.Fatalf("OnStart() error = %v, want nil", err)
	}

	registry.waitRegistered(t)
	manager.waitStarted(t)

	if got := registry.registerAllCalls(); got != 1 {
		t.Fatalf("RegisterAll calls = %d, want 1", got)
	}
	if got := manager.startedMux(); got == nil {
		t.Fatalf("StartServer mux = nil, want non-nil")
	}
	if got := manager.startedMux(); got != registry.lastMux() {
		t.Fatalf("StartServer mux = %p, want %p", got, registry.lastMux())
	}

	if err := lc.hooks[0].OnStop(context.Background()); err != nil {
		t.Fatalf("OnStop() error = %v, want nil", err)
	}

	if got := manager.stopCount(); got != 1 {
		t.Fatalf("StopServer calls = %d, want 1", got)
	}
	if got := manager.closeCount(); got != 1 {
		t.Fatalf("Close calls = %d, want 1", got)
	}
}

func TestRegisterSchedulerLifecycle(t *testing.T) {
	t.Parallel()

	lc := &recordingLifecycle{}
	scheduler := &fakeScheduler{}
	log := newTestLogger(t)

	fxapp.RegisterSchedulerLifecycle(lc, scheduler, log)

	if got := len(lc.hooks); got != 1 {
		t.Fatalf("hooks len = %d, want 1", got)
	}

	if err := lc.hooks[0].OnStart(context.Background()); err != nil {
		t.Fatalf("OnStart() error = %v, want nil", err)
	}
	if got := scheduler.startCount(); got != 1 {
		t.Fatalf("Start calls = %d, want 1", got)
	}

	if err := lc.hooks[0].OnStop(context.Background()); err != nil {
		t.Fatalf("OnStop() error = %v, want nil", err)
	}
	if got := scheduler.stopCount(); got != 1 {
		t.Fatalf("Stop calls = %d, want 1", got)
	}
}

type recordingLifecycle struct {
	hooks []fx.Hook
}

func (l *recordingLifecycle) Append(h fx.Hook) {
	l.hooks = append(l.hooks, h)
}

type fakeGRPCServer struct {
	mu         sync.Mutex
	started    chan struct{}
	startedAt  net.Listener
	startCalls int
	stopCalls  int
}

func newFakeGRPCServer() *fakeGRPCServer {
	return &fakeGRPCServer{started: make(chan struct{})}
}

func (s *fakeGRPCServer) Serve(listener net.Listener) error {
	s.mu.Lock()
	s.startedAt = listener
	s.startCalls++
	if s.started == nil {
		s.started = make(chan struct{})
	}
	close(s.started)
	s.mu.Unlock()
	return nil
}

func (s *fakeGRPCServer) Stop() {
	s.mu.Lock()
	s.stopCalls++
	s.mu.Unlock()
}

func (s *fakeGRPCServer) waitStarted(t *testing.T) {
	t.Helper()

	s.mu.Lock()
	ch := s.started
	s.mu.Unlock()

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for gRPC server start")
	}
}

func (s *fakeGRPCServer) startedListener() net.Listener {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startedAt
}

func (s *fakeGRPCServer) startCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startCalls
}

func (s *fakeGRPCServer) stopCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopCalls
}

type fakeTaskHandlerRegistrar struct {
	mu         sync.Mutex
	registers  int
	lastMuxRef *asynq.ServeMux
	registered chan struct{}
}

func newFakeTaskHandlerRegistrar() *fakeTaskHandlerRegistrar {
	return &fakeTaskHandlerRegistrar{registered: make(chan struct{})}
}

func (r *fakeTaskHandlerRegistrar) RegisterAll(mux *asynq.ServeMux) {
	r.mu.Lock()
	r.registers++
	r.lastMuxRef = mux
	if r.registered == nil {
		r.registered = make(chan struct{})
	}
	close(r.registered)
	r.mu.Unlock()
}

func (r *fakeTaskHandlerRegistrar) waitRegistered(t *testing.T) {
	t.Helper()

	r.mu.Lock()
	ch := r.registered
	r.mu.Unlock()

	if ch == nil {
		t.Fatal("register channel is nil")
	}

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for task handler registration")
	}
}

func (r *fakeTaskHandlerRegistrar) registerAllCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.registers
}

func (r *fakeTaskHandlerRegistrar) lastMux() *asynq.ServeMux {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastMuxRef
}

type fakeTaskManager struct {
	mu         sync.Mutex
	started    chan struct{}
	mux        *asynq.ServeMux
	startCalls int
	stopCalls  int
	closeCalls int
}

func newFakeTaskManager() *fakeTaskManager {
	return &fakeTaskManager{started: make(chan struct{})}
}

func (m *fakeTaskManager) StartServer(mux *asynq.ServeMux) error {
	m.mu.Lock()
	m.mux = mux
	m.startCalls++
	if m.started == nil {
		m.started = make(chan struct{})
	}
	close(m.started)
	m.mu.Unlock()
	return nil
}

func (m *fakeTaskManager) StopServer() {
	m.mu.Lock()
	m.stopCalls++
	m.mu.Unlock()
}

func (m *fakeTaskManager) Close() error {
	m.mu.Lock()
	m.closeCalls++
	m.mu.Unlock()
	return nil
}

func (m *fakeTaskManager) waitStarted(t *testing.T) {
	t.Helper()

	m.mu.Lock()
	ch := m.started
	m.mu.Unlock()

	if ch == nil {
		t.Fatal("start channel is nil")
	}

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for task manager start")
	}
}

func (m *fakeTaskManager) startedMux() *asynq.ServeMux {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mux
}

func (m *fakeTaskManager) stopCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopCalls
}

func (m *fakeTaskManager) closeCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closeCalls
}

type fakeScheduler struct {
	mu     sync.Mutex
	starts int
	stops  int
}

func (s *fakeScheduler) Start() error {
	s.mu.Lock()
	s.starts++
	s.mu.Unlock()
	return nil
}

func (s *fakeScheduler) Stop() error {
	s.mu.Lock()
	s.stops++
	s.mu.Unlock()
	return nil
}

func (s *fakeScheduler) startCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.starts
}

func (s *fakeScheduler) stopCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stops
}

func newTestLogger(t *testing.T) *zap.Logger {
	t.Helper()

	log, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("zap.NewDevelopment() error = %v", err)
	}
	t.Cleanup(func() {
		_ = log.Sync()
	})
	return log
}

type fakeListener struct {
	mu          sync.Mutex
	addr        net.Addr
	closeCountV int
}

func newFakeListener(address string) *fakeListener {
	return &fakeListener{addr: stringAddr(address)}
}

func (l *fakeListener) Accept() (net.Conn, error) {
	return nil, net.ErrClosed
}

func (l *fakeListener) Close() error {
	l.mu.Lock()
	l.closeCountV++
	l.mu.Unlock()
	return nil
}

func (l *fakeListener) Addr() net.Addr {
	return l.addr
}

func (l *fakeListener) closeCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.closeCountV
}

type stringAddr string

func (a stringAddr) Network() string { return "tcp" }

func (a stringAddr) String() string { return string(a) }
