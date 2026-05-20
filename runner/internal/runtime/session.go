package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/zgiai/zgi/runner/internal/invoke"
	"github.com/zgiai/zgi/runner/internal/plugin"
	"github.com/zgiai/zgi/runner/internal/protocol"
)

// Session tracks the lifecycle of one plugin process.
type Session struct {
	id         string
	manifest   plugin.Manifest
	workingDir string

	mu             sync.RWMutex
	status         SessionStatus
	startedAt      time.Time
	finished       *time.Time
	lastActivityAt time.Time
	pid            int
	errMsg         string
	metadata       SessionMetadata
	hasMetadata    bool

	stopFn func(context.Context) error
	logs   *logBuffer
	done   chan error

	// Router for request/response management
	router *invoke.Router
	writer func([]byte) error
}

// NewSession constructs a session descriptor.
func NewSession(manifest plugin.Manifest, workingDir string) *Session {
	now := time.Now()
	s := &Session{
		id:             uuid.NewString(),
		manifest:       manifest,
		workingDir:     workingDir,
		status:         SessionStatusLaunching,
		startedAt:      now,
		lastActivityAt: now,
		logs:           newLogBuffer(200),
		done:           make(chan error, 1),
	}
	return s
}

// ID returns the session identifier.
func (s *Session) ID() string {
	return s.id
}

// Done exposes the completion channel.
func (s *Session) Done() <-chan error {
	return s.done
}

// Wait blocks until the plugin exits.
func (s *Session) Wait() error {
	return <-s.done
}

// Stop sends the runtime-specific stop signal.
func (s *Session) Stop(ctx context.Context) error {
	s.mu.RLock()
	fn := s.stopFn
	s.mu.RUnlock()
	if fn == nil {
		return errors.New("stop function not configured")
	}
	return fn(ctx)
}

// SetStopFunc configures how the runtime should terminate the process.
func (s *Session) SetStopFunc(fn func(context.Context) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopFn = fn
}

// MarkRunning updates status and pid once the process is alive.
func (s *Session) MarkRunning(pid int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = SessionStatusRunning
	s.pid = pid
}

// MarkExited stores the exit status.
func (s *Session) MarkExited(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.status = SessionStatusFailed
		s.errMsg = err.Error()
	} else {
		s.status = SessionStatusExited
	}
	now := time.Now()
	s.finished = &now

	select {
	case s.done <- err:
	default:
	}
}

// AppendLog stores a stdout/stderr line in the ring buffer.
func (s *Session) AppendLog(stream, line string) {
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return
	}
	s.logs.append(LogLine{
		Timestamp: time.Now(),
		Stream:    stream,
		Line:      line,
	})
}

// Snapshot returns a snapshot of the session.
func (s *Session) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snap := Snapshot{
		ID:         s.id,
		Manifest:   s.manifest,
		WorkingDir: s.workingDir,
		Status:     s.status,
		StartedAt:  s.startedAt,
		PID:        s.pid,
		Error:      s.errMsg,
		Logs:       s.logs.snapshot(),
	}
	if s.finished != nil {
		f := *s.finished
		snap.FinishedAt = &f
	}
	if s.hasMetadata {
		meta := s.metadata
		snap.Metadata = &meta
	}
	if !s.lastActivityAt.IsZero() {
		lastActivity := s.lastActivityAt
		snap.LastActivityAt = &lastActivity
	}
	return snap
}

// SetMetadata stores optional session metadata used for lifecycle management.
func (s *Session) SetMetadata(meta SessionMetadata) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metadata = meta
	s.hasMetadata = !isZeroSessionMetadata(meta)
}

// TouchActivity updates the last activity timestamp.
func (s *Session) TouchActivity() {
	s.mu.Lock()
	s.lastActivityAt = time.Now()
	s.mu.Unlock()
}

// SetError allows the runtime to record an error message without exiting yet.
func (s *Session) SetError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errMsg = err.Error()
}

// SetWriter sets the stdin writer for the plugin process.
func (s *Session) SetWriter(writer func([]byte) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writer = writer
	s.router = invoke.NewRouter(s.id, writer)
}

// Router returns the request router for this session.
func (s *Session) Router() *invoke.Router {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.router
}

// Write sends data to the plugin's stdin.
func (s *Session) Write(data []byte) error {
	s.mu.RLock()
	writer := s.writer
	s.mu.RUnlock()
	if writer == nil {
		return errors.New("writer not configured")
	}
	return writer(data)
}

// SendRequest sends a request to the plugin.
func (s *Session) SendRequest(ctx context.Context, req *protocol.Request, timeout time.Duration) (*protocol.Message, error) {
	return s.SendRequestWithMode(ctx, req, timeout, string(invoke.WaitModeFirst), string(invoke.StreamModeFirst))
}

// SendRequestWithMode sends a request using wait/stream mode configuration.
func (s *Session) SendRequestWithMode(
	ctx context.Context,
	req *protocol.Request,
	timeout time.Duration,
	waitMode string,
	streamMode string,
) (*protocol.Message, error) {
	s.mu.RLock()
	router := s.router
	s.mu.RUnlock()
	if router == nil {
		return nil, errors.New("router not configured")
	}
	s.TouchActivity()
	return router.SendSyncWithMode(ctx, req, timeout, invoke.WaitMode(waitMode), invoke.StreamMode(streamMode))
}

// SetCallbackHandler sets the handler for plugin callbacks.
func (s *Session) SetCallbackHandler(handler invoke.CallbackHandler) {
	s.mu.RLock()
	router := s.router
	s.mu.RUnlock()
	if router != nil {
		router.SetCallbackHandler(handler)
	}
}

// HandleMessage routes an incoming message from the plugin.
func (s *Session) HandleMessage(msg *protocol.Message) error {
	s.mu.RLock()
	router := s.router
	s.mu.RUnlock()
	if router == nil {
		return errors.New("router not configured")
	}
	return router.HandleMessage(msg)
}

// WaitReady blocks until the plugin signals readiness.
func (s *Session) WaitReady(ctx context.Context) error {
	s.mu.RLock()
	router := s.router
	s.mu.RUnlock()
	if router == nil {
		return errors.New("router not configured")
	}
	return router.WaitReady(ctx)
}

// IsReady returns whether the plugin is ready.
func (s *Session) IsReady() bool {
	s.mu.RLock()
	router := s.router
	s.mu.RUnlock()
	if router == nil {
		return false
	}
	return router.IsReady()
}

// FailFast marks the session as failed and closes the channel immediately.
func (s *Session) FailFast(err error) {
	if err == nil {
		err = fmt.Errorf("runtime failed without error detail")
	}
	s.mu.Lock()
	s.status = SessionStatusFailed
	s.errMsg = err.Error()
	now := time.Now()
	s.finished = &now
	s.mu.Unlock()

	select {
	case s.done <- err:
	default:
	}
}

func isZeroSessionMetadata(meta SessionMetadata) bool {
	return meta.WorkflowRunID == "" &&
		meta.SessionPolicy == "" &&
		meta.SessionIdleTTLSeconds == 0 &&
		meta.SessionMaxLifetimeSeconds == 0
}
