package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"go.uber.org/zap"

	"github.com/zgiai/zgi/runner/internal/ipc"
	"github.com/zgiai/zgi/runner/internal/plugin"
	"github.com/zgiai/zgi/runner/internal/protocol"
	"github.com/zgiai/zgi/runner/internal/runtime"
)

// Config represents knobs for the local runtime.
type Config struct {
	PythonInterpreter   string
	NodeInterpreter     string
	StdoutBufferSize    int
	StdoutMaxBufferSize int
	ShutdownGracePeriod time.Duration
	HTTPProxy           string
	HTTPSProxy          string
	NoProxy             string
}

// Runtime launches plugins as local subprocesses.
type Runtime struct {
	cfg Config
	log *zap.Logger
}

// New creates the local runtime.
func New(cfg Config, log *zap.Logger) *Runtime {
	return &Runtime{
		cfg: cfg,
		log: log,
	}
}

// Start implements runtime.Runtime.
func (r *Runtime) Start(_ context.Context, req runtime.StartRequest) (*runtime.Session, error) {
	if err := req.Manifest.Validate(); err != nil {
		return nil, err
	}
	if req.WorkingDir == "" {
		return nil, fmt.Errorf("working directory is required")
	}

	session := runtime.NewSession(req.Manifest, req.WorkingDir)

	cmd, stdin, stdout, stderr, err := r.buildCommand(req)
	if err != nil {
		session.FailFast(err)
		return nil, err
	}

	session.SetStopFunc(func(stopCtx context.Context) error {
		return r.terminate(stopCtx, cmd, session)
	})

	if err := cmd.Start(); err != nil {
		session.FailFast(err)
		return nil, fmt.Errorf("start plugin: %w", err)
	}

	session.MarkRunning(cmd.Process.Pid)
	r.log.Info("plugin started",
		zap.String("session_id", session.ID()),
		zap.String("plugin", req.Manifest.Name),
		zap.Int("pid", cmd.Process.Pid),
	)

	// Setup bidirectional stdio communication
	stdio := ipc.NewStdIO(stdin, stdout, stderr, r.cfg.StdoutBufferSize, r.cfg.StdoutMaxBufferSize)

	// Configure writer for the session
	session.SetWriter(func(data []byte) error {
		return stdio.Write(data)
	})

	// Start reading stdout/stderr in background
	go func() {
		stdio.StartReading(
			// Message handler - route protocol messages
			func(msg *protocol.Message) error {
				return session.HandleMessage(msg)
			},
			// Log handler - append non-protocol output to logs
			func(stream, line string) {
				session.AppendLog(stream, line)
			},
		)
	}()

	go func() {
		err := cmd.Wait()
		// Close the router to cancel pending requests
		if router := session.Router(); router != nil {
			router.Close()
		}
		session.MarkExited(err)
		if err != nil {
			r.log.Warn("plugin exited with error",
				zap.String("session_id", session.ID()),
				zap.Error(err),
			)
		} else {
			r.log.Info("plugin exited", zap.String("session_id", session.ID()))
		}
	}()

	return session, nil
}

func (r *Runtime) buildCommand(req runtime.StartRequest) (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
	var args []string
	var interpreter string
	switch req.Manifest.Runner.Language {
	case plugin.LanguagePython:
		// Prefer virtualenv Python if it exists
		venvPython := filepath.Join(req.WorkingDir, ".venv", "bin", "python")
		if _, err := os.Stat(venvPython); err == nil {
			interpreter = venvPython
		} else {
			interpreter = r.cfg.PythonInterpreter
		}
		args = []string{"-m", req.Manifest.Runner.Entrypoint}
	case plugin.LanguageNode:
		interpreter = r.cfg.NodeInterpreter
		args = []string{req.Manifest.Runner.Entrypoint}
	default:
		return nil, nil, nil, nil, fmt.Errorf("language %q is not supported", req.Manifest.Runner.Language)
	}
	if len(req.Args) > 0 {
		args = append(args, req.Args...)
	}

	cmd := exec.CommandContext(context.Background(), interpreter, args...)
	cmd.Dir = req.WorkingDir
	env := append(os.Environ(), formatEnv(req.Env)...)
	env = append(env, r.proxyEnv()...)
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("stderr pipe: %w", err)
	}

	return cmd, stdin, stdout, stderr, nil
}

func (r *Runtime) proxyEnv() []string {
	var env []string
	if r.cfg.HTTPProxy != "" {
		env = append(env, fmt.Sprintf("HTTP_PROXY=%s", r.cfg.HTTPProxy))
	}
	if r.cfg.HTTPSProxy != "" {
		env = append(env, fmt.Sprintf("HTTPS_PROXY=%s", r.cfg.HTTPSProxy))
	}
	if r.cfg.NoProxy != "" {
		env = append(env, fmt.Sprintf("NO_PROXY=%s", r.cfg.NoProxy))
	}
	return env
}

func (r *Runtime) terminate(ctx context.Context, cmd *exec.Cmd, session *runtime.Session) error {
	if cmd.Process == nil {
		return nil
	}

	if err := cmd.Process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
		r.log.Warn("failed to interrupt process, killing",
			zap.String("session_id", session.ID()),
			zap.Error(err),
		)
		return cmd.Process.Kill()
	}

	timeout := r.cfg.ShutdownGracePeriod
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	select {
	case err := <-session.Done():
		return err
	case <-ctx.Done():
		return cmd.Process.Kill()
	case <-time.After(timeout):
		r.log.Warn("graceful shutdown timed out, killing process", zap.String("session_id", session.ID()))
		return cmd.Process.Kill()
	}
}

func formatEnv(vars map[string]string) []string {
	if len(vars) == 0 {
		return nil
	}
	out := make([]string, 0, len(vars))
	for k, v := range vars {
		out = append(out, fmt.Sprintf("%s=%s", k, v))
	}
	return out
}
