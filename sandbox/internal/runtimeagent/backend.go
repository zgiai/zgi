// Package runtimeagent implements the narrow execution protocol that runs
// inside an already isolated Linux environment. It does not create a security
// boundary by itself; the outer worker must provide gVisor, VM, or equivalent
// isolation and enforce the advertised network policy.
package runtimeagent

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

type Options struct {
	WorkspaceRoot    string
	NetworkMode      string
	MaxUploadBytes   int64
	MaxDownloadBytes int64
}

type Backend struct {
	workspace        *os.Root
	workspaceName    string
	networkMode      string
	maxUploadBytes   int64
	maxDownloadBytes int64
	mu               sync.Mutex
	jobs             map[string]trackedJob
	closed           bool
}

type trackedJob struct {
	specHash string
	running  agentexecution.RunningJob
}

var _ agentexecution.ExecutionBackend = (*Backend)(nil)

func New(options Options) (*Backend, error) {
	options.WorkspaceRoot = strings.TrimSpace(options.WorkspaceRoot)
	if options.WorkspaceRoot == "" {
		return nil, fmt.Errorf("%w: workspace root is required", agentexecution.ErrInvalidRequest)
	}
	if err := os.MkdirAll(options.WorkspaceRoot, 0o750); err != nil {
		return nil, fmt.Errorf("prepare runtime workspace: %w", err)
	}
	root, err := os.OpenRoot(options.WorkspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("open runtime workspace: %w", err)
	}
	if options.MaxUploadBytes <= 0 {
		options.MaxUploadBytes = 64 * 1024 * 1024
	}
	if options.MaxDownloadBytes <= 0 {
		options.MaxDownloadBytes = 64 * 1024 * 1024
	}
	mode := strings.TrimSpace(options.NetworkMode)
	if mode == "" {
		mode = "unknown"
	}
	return &Backend{
		workspace: root, workspaceName: root.Name(), networkMode: mode,
		maxUploadBytes: options.MaxUploadBytes, maxDownloadBytes: options.MaxDownloadBytes,
		jobs: make(map[string]trackedJob),
	}, nil
}

func (b *Backend) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	jobs := make([]agentexecution.RunningJob, 0, len(b.jobs))
	for _, job := range b.jobs {
		jobs = append(jobs, job.running)
	}
	b.mu.Unlock()
	for _, job := range jobs {
		_ = job.Cancel(context.Background())
		waitContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _ = job.Wait(waitContext)
		cancel()
	}
	return b.workspace.Close()
}

func (b *Backend) Capabilities(context.Context, agentexecution.Allocation) (agentexecution.RuntimeCapabilities, error) {
	return agentexecution.RuntimeCapabilities{
		Shell: true, PTY: supportsPTY(), FileTransfer: true, NetworkMode: b.networkMode,
		MaxUploadBytes: b.maxUploadBytes, MaxDownloadBytes: b.maxDownloadBytes,
	}, nil
}

func (b *Backend) StartJob(ctx context.Context, _ agentexecution.Allocation, spec agentexecution.JobSpec, sink func(agentexecution.OutputStream, []byte) error) (agentexecution.RunningJob, error) {
	if err := validateJobSpec(spec, sink); err != nil {
		return nil, err
	}
	workDir, err := b.resolveWorkDir(spec.WorkDir)
	if err != nil {
		return nil, err
	}
	hash, err := hashJobSpec(spec)
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, agentexecution.ErrBackendUnavailable
	}
	if existing, ok := b.jobs[spec.JobID]; ok {
		if existing.specHash != hash {
			return nil, agentexecution.ErrIdempotencyConflict
		}
		return existing.running, nil
	}
	running, err := startProcess(ctx, workDir, spec, sink)
	if err != nil {
		return nil, err
	}
	b.jobs[spec.JobID] = trackedJob{specHash: hash, running: running}
	return running, nil
}

func validateJobSpec(spec agentexecution.JobSpec, sink func(agentexecution.OutputStream, []byte) error) error {
	if strings.TrimSpace(spec.JobID) == "" || strings.TrimSpace(spec.Command) == "" || sink == nil {
		return fmt.Errorf("%w: job id, command, and output sink are required", agentexecution.ErrInvalidRequest)
	}
	if strings.ContainsRune(spec.Command, '\x00') {
		return fmt.Errorf("%w: command contains a null byte", agentexecution.ErrInvalidRequest)
	}
	for _, argument := range spec.Args {
		if strings.ContainsRune(argument, '\x00') {
			return fmt.Errorf("%w: argument contains a null byte", agentexecution.ErrInvalidRequest)
		}
	}
	for key, value := range spec.Environment {
		if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("%w: environment contains an invalid entry", agentexecution.ErrInvalidRequest)
		}
	}
	if spec.PTY && (spec.PTYColumns == 0 || spec.PTYRows == 0) {
		return fmt.Errorf("%w: PTY dimensions are required", agentexecution.ErrInvalidRequest)
	}
	if spec.Timeout < 0 {
		return fmt.Errorf("%w: timeout cannot be negative", agentexecution.ErrInvalidRequest)
	}
	return nil
}

func (b *Backend) UploadFile(ctx context.Context, _ agentexecution.Allocation, filePath string, reader io.Reader, size int64) (agentexecution.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return agentexecution.FileInfo{}, err
	}
	clean, err := cleanRelativePath(filePath, false)
	if err != nil {
		return agentexecution.FileInfo{}, err
	}
	if reader == nil || size < 0 || size > b.maxUploadBytes {
		return agentexecution.FileInfo{}, agentexecution.ErrOutputLimit
	}
	parent := path.Dir(clean)
	if parent != "." {
		if err := b.workspace.MkdirAll(parent, 0o750); err != nil {
			return agentexecution.FileInfo{}, fmt.Errorf("prepare upload directory: %w", err)
		}
	}
	temporary, err := temporaryName(clean)
	if err != nil {
		return agentexecution.FileInfo{}, err
	}
	file, err := b.workspace.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return agentexecution.FileInfo{}, fmt.Errorf("create upload: %w", err)
	}
	keep := false
	defer func() {
		_ = file.Close()
		if !keep {
			_ = b.workspace.Remove(temporary)
		}
	}()
	written, copyErr := io.CopyN(file, reader, size)
	if copyErr != nil {
		return agentexecution.FileInfo{}, fmt.Errorf("write upload after %d bytes: %w", written, copyErr)
	}
	if err := file.Sync(); err != nil {
		return agentexecution.FileInfo{}, fmt.Errorf("sync upload: %w", err)
	}
	if err := file.Close(); err != nil {
		return agentexecution.FileInfo{}, fmt.Errorf("close upload: %w", err)
	}
	if err := b.workspace.Rename(temporary, clean); err != nil {
		return agentexecution.FileInfo{}, fmt.Errorf("commit upload: %w", err)
	}
	keep = true
	return b.fileInfo(clean)
}

func (b *Backend) DownloadFile(ctx context.Context, _ agentexecution.Allocation, filePath string) (io.ReadCloser, agentexecution.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, agentexecution.FileInfo{}, err
	}
	clean, err := cleanRelativePath(filePath, false)
	if err != nil {
		return nil, agentexecution.FileInfo{}, err
	}
	info, err := b.fileInfo(clean)
	if err != nil {
		return nil, agentexecution.FileInfo{}, err
	}
	if info.Size > b.maxDownloadBytes {
		return nil, agentexecution.FileInfo{}, agentexecution.ErrOutputLimit
	}
	file, err := b.workspace.Open(clean)
	if err != nil {
		return nil, agentexecution.FileInfo{}, fmt.Errorf("open download: %w", err)
	}
	return file, info, nil
}

func (b *Backend) fileInfo(filePath string) (agentexecution.FileInfo, error) {
	info, err := b.workspace.Stat(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return agentexecution.FileInfo{}, agentexecution.ErrNotFound
		}
		return agentexecution.FileInfo{}, err
	}
	if !info.Mode().IsRegular() {
		return agentexecution.FileInfo{}, fmt.Errorf("%w: workspace path is not a regular file", agentexecution.ErrInvalidPath)
	}
	return agentexecution.FileInfo{
		Path: filePath, Size: info.Size(), Mode: uint32(info.Mode().Perm()),
		ModifiedAt: info.ModTime().UTC(), ContentType: mime.TypeByExtension(path.Ext(filePath)),
	}, nil
}

func (b *Backend) resolveWorkDir(value string) (string, error) {
	if strings.TrimSpace(value) == "" || strings.TrimSpace(value) == "." {
		return b.workspaceName, nil
	}
	clean, err := cleanRelativePath(value, true)
	if err != nil {
		return "", err
	}
	info, err := b.workspace.Stat(clean)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: work directory does not exist", agentexecution.ErrInvalidPath)
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: work directory is not a directory", agentexecution.ErrInvalidPath)
	}
	return filepath.Join(b.workspaceName, filepath.FromSlash(clean)), nil
}

func cleanRelativePath(value string, allowDot bool) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || strings.ContainsRune(value, '\x00') || strings.HasPrefix(value, "/") || (len(value) >= 2 && value[1] == ':') {
		return "", agentexecution.ErrInvalidPath
	}
	clean := path.Clean(value)
	if clean == ".." || strings.HasPrefix(clean, "../") || (!allowDot && clean == ".") {
		return "", agentexecution.ErrInvalidPath
	}
	return clean, nil
}

func temporaryName(target string) (string, error) {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return path.Join(path.Dir(target), "."+path.Base(target)+".upload-"+hex.EncodeToString(value[:])), nil
}

func hashJobSpec(spec agentexecution.JobSpec) (string, error) {
	payload, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	var value [32]byte
	value = sha256Sum(payload)
	return hex.EncodeToString(value[:]), nil
}

func sha256Sum(value []byte) [32]byte {
	return sha256.Sum256(value)
}
