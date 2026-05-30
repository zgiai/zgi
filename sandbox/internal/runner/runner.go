package runner

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/config"
)

type Service struct {
	semaphore chan struct{}
	timeout   time.Duration
	outputCap int
	backend   backend
}

type Request struct {
	Language      string `json:"language"`
	Code          string `json:"code"`
	Preload       string `json:"preload"`
	Stdin         string `json:"stdin,omitempty"`
	EnableNetwork bool   `json:"enable_network"`
}

type Result struct {
	Stdout           string   `json:"stdout"`
	Error            string   `json:"error"`
	ExitCode         int      `json:"exit_code"`
	DurationMS       int64    `json:"duration_ms"`
	NetworkRequested bool     `json:"network_requested"`
	Truncated        bool     `json:"truncated"`
	Backend          string   `json:"backend,omitempty"`
	ResultJSON       any      `json:"result_json,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
}

type CommandResult struct {
	Stdout     string   `json:"stdout"`
	Error      string   `json:"error"`
	ExitCode   int      `json:"exit_code"`
	DurationMS int64    `json:"duration_ms"`
	Truncated  bool     `json:"truncated"`
	Command    string   `json:"command"`
	Args       []string `json:"args,omitempty"`
	Backend    string   `json:"backend,omitempty"`
}

type CommandSpec struct {
	WorkDir        string
	Command        string
	Args           []string
	Stdin          string
	Env            map[string]string
	Timeout        time.Duration
	StdoutLimit    int
	StderrLimit    int
	AllowShellForm bool
}

type Options struct {
	MaxWorkers int
	Timeout    time.Duration
	OutputCap  int
	Backend    backend
}

type runtimeSpec struct {
	binary   string
	filename string
	args     func(scriptPath string) []string
}

type backend interface {
	Name() string
	Run(context.Context, Request, string, bool, time.Duration, int, int) (Result, error)
	ExecuteCommand(context.Context, CommandSpec) (CommandResult, error)
}

func NewService(maxWorkers int, timeout time.Duration, outputCap int) *Service {
	return NewServiceWithOptions(Options{
		MaxWorkers: maxWorkers,
		Timeout:    timeout,
		OutputCap:  outputCap,
		Backend:    newProcessBackend(),
	})
}

func NewServiceWithOptions(options Options) *Service {
	if options.MaxWorkers <= 0 {
		options.MaxWorkers = 1
	}
	if options.OutputCap <= 0 {
		options.OutputCap = 64 * 1024
	}
	if options.Timeout <= 0 {
		options.Timeout = 5 * time.Second
	}
	if options.Backend == nil {
		options.Backend = newProcessBackend()
	}

	return &Service{
		semaphore: make(chan struct{}, options.MaxWorkers),
		timeout:   options.Timeout,
		outputCap: options.OutputCap,
		backend:   options.Backend,
	}
}

func NewServiceFromConfig(cfg config.Config) (*Service, error) {
	backend, err := newBackendFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	return NewServiceWithOptions(Options{
		MaxWorkers: cfg.MaxWorkers,
		Timeout:    time.Duration(cfg.TimeoutSeconds) * time.Second,
		OutputCap:  cfg.OutputLimitKB * 1024,
		Backend:    backend,
	}), nil
}

func (s *Service) Run(parent context.Context, req Request) (Result, error) {
	return s.run(parent, req, "", true, s.timeout, s.outputCap, s.outputCap)
}

func (s *Service) RunWithLimits(parent context.Context, req Request, timeout time.Duration, stdoutLimit int, stderrLimit int) (Result, error) {
	return s.run(parent, req, "", true, timeout, stdoutLimit, stderrLimit)
}

func (s *Service) RunInDir(parent context.Context, req Request, workDir string) (Result, error) {
	return s.run(parent, req, workDir, false, s.timeout, s.outputCap, s.outputCap)
}

func (s *Service) RunInDirWithLimits(parent context.Context, req Request, workDir string, timeout time.Duration, stdoutLimit int, stderrLimit int) (Result, error) {
	return s.run(parent, req, workDir, false, timeout, stdoutLimit, stderrLimit)
}

func (s *Service) run(parent context.Context, req Request, workDir string, ephemeral bool, timeout time.Duration, stdoutLimit int, stderrLimit int) (Result, error) {
	if strings.TrimSpace(req.Code) == "" {
		return Result{}, errors.New("code is required")
	}
	if _, err := languageSpec(req.Language); err != nil {
		return Result{}, err
	}

	select {
	case s.semaphore <- struct{}{}:
		defer func() { <-s.semaphore }()
	case <-parent.Done():
		return Result{}, parent.Err()
	}

	if timeout <= 0 {
		timeout = s.timeout
	}
	if stdoutLimit <= 0 {
		stdoutLimit = s.outputCap
	}
	if stderrLimit <= 0 {
		stderrLimit = s.outputCap
	}

	result, err := s.backend.Run(parent, req, workDir, ephemeral, timeout, stdoutLimit, stderrLimit)
	if err != nil {
		return Result{}, err
	}
	result.Backend = s.backend.Name()
	return result, nil
}

func (s *Service) ExecuteCommand(parent context.Context, workDir string, command string, args []string, timeout time.Duration) (CommandResult, error) {
	return s.ExecuteCommandSpec(parent, CommandSpec{
		WorkDir:        workDir,
		Command:        command,
		Args:           args,
		Timeout:        timeout,
		StdoutLimit:    s.outputCap,
		StderrLimit:    s.outputCap,
		AllowShellForm: true,
	})
}

func (s *Service) ExecuteCommandSpec(parent context.Context, spec CommandSpec) (CommandResult, error) {
	if strings.TrimSpace(spec.Command) == "" {
		return CommandResult{}, errors.New("command is required")
	}
	if spec.WorkDir == "" {
		return CommandResult{}, errors.New("working directory is required")
	}

	select {
	case s.semaphore <- struct{}{}:
		defer func() { <-s.semaphore }()
	case <-parent.Done():
		return CommandResult{}, parent.Err()
	}

	if spec.Timeout <= 0 {
		spec.Timeout = s.timeout
	}
	if spec.StdoutLimit <= 0 {
		spec.StdoutLimit = s.outputCap
	}
	if spec.StderrLimit <= 0 {
		spec.StderrLimit = s.outputCap
	}

	result, err := s.backend.ExecuteCommand(parent, spec)
	if err != nil {
		return CommandResult{}, err
	}
	result.Backend = s.backend.Name()
	return result, nil
}

func languageSpec(language string) (runtimeSpec, error) {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "python", "python3":
		return runtimeSpec{
			binary:   "python3",
			filename: "main.py",
			args: func(scriptPath string) []string {
				return []string{scriptPath}
			},
		}, nil
	case "node", "nodejs", "javascript":
		return runtimeSpec{
			binary:   "node",
			filename: "main.js",
			args: func(scriptPath string) []string {
				return []string{scriptPath}
			},
		}, nil
	default:
		return runtimeSpec{}, fmt.Errorf("unsupported language: %s", language)
	}
}

func buildContent(preload string, code string) string {
	parts := make([]string, 0, 2)
	if strings.TrimSpace(preload) != "" {
		parts = append(parts, strings.TrimSpace(preload))
	}
	parts = append(parts, code)
	return strings.Join(parts, "\n\n")
}

func token() string {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}

func containerScriptPath(workDir string, filename string) string {
	return filepath.ToSlash(filepath.Join("/tmp/workspace", filename))
}

type cappedBuffer struct {
	limit     int
	buf       []byte
	truncated bool
}

func newCappedBuffer(limit int) *cappedBuffer {
	return &cappedBuffer{
		limit: limit,
		buf:   make([]byte, 0, limit),
	}
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		return len(p), nil
	}

	remaining := b.limit - len(b.buf)
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}

	if len(p) > remaining {
		b.buf = append(b.buf, p[:remaining]...)
		b.truncated = true
		return len(p), nil
	}

	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *cappedBuffer) String() string {
	return string(b.buf)
}

func (b *cappedBuffer) Truncated() bool {
	return b.truncated
}

func (b *cappedBuffer) AppendLine(message string) {
	if message == "" {
		return
	}
	if !strings.HasSuffix(message, "\n") {
		message += "\n"
	}
	_, _ = b.Write([]byte(message))
}

func (b *cappedBuffer) Bytes() []byte {
	return bytes.Clone(b.buf)
}

func safeBaseEnv(values []string) []string {
	safe := make([]string, 0, len(values))
	for _, item := range values {
		key, _, ok := strings.Cut(item, "=")
		if !ok || unsafeBaseEnvKey(key) {
			continue
		}
		safe = append(safe, item)
	}
	return safe
}

func unsafeBaseEnvKey(key string) bool {
	upper := strings.ToUpper(key)
	switch upper {
	case "IFS", "SHELLOPTS", "BASH_ENV", "ENV", "PYTHONPATH", "NODE_OPTIONS":
		return true
	default:
		return strings.HasPrefix(upper, "LD_") || strings.HasPrefix(upper, "DYLD_")
	}
}
