package executor

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/lifecycle"
	"github.com/zgiai/zgi-sandbox/internal/observer"
	"github.com/zgiai/zgi-sandbox/internal/policy"
	"github.com/zgiai/zgi-sandbox/internal/runner"
)

type Service struct {
	lifecycle *lifecycle.Manager
	runner    *runner.Service
	observer  *observer.Recorder
	policy    *policy.Service
}

type CodeRequest struct {
	SandboxID     string `json:"sandbox_id"`
	Language      string `json:"language"`
	Code          string `json:"code"`
	Preload       string `json:"preload"`
	EnableNetwork bool   `json:"enable_network"`
}

type CommandRequest struct {
	SandboxID      string   `json:"sandbox_id"`
	Command        string   `json:"command"`
	Args           []string `json:"args"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	WorkingSubpath string   `json:"working_subpath"`
}

type FileWriteRequest struct {
	SandboxID string `json:"sandbox_id"`
	Path      string `json:"path"`
	Content   string `json:"content"`
	Encoding  string `json:"encoding"`
}

type FileInfo struct {
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	Mode        string    `json:"mode"`
	ModifiedAt  time.Time `json:"modified_at"`
	IsDirectory bool      `json:"is_directory"`
	SandboxID   string    `json:"sandbox_id"`
}

type FileContent struct {
	SandboxID string `json:"sandbox_id"`
	Path      string `json:"path"`
	Content   string `json:"content"`
	Encoding  string `json:"encoding"`
}

func NewService(manager *lifecycle.Manager, runnerService *runner.Service, recorder *observer.Recorder, policyService *policy.Service) *Service {
	return &Service{
		lifecycle: manager,
		runner:    runnerService,
		observer:  recorder,
		policy:    policyService,
	}
}

func (s *Service) RunCode(ctx context.Context, req CodeRequest) (runner.Result, error) {
	box, err := s.lifecycle.GetActive(req.SandboxID)
	if err != nil {
		return runner.Result{}, err
	}
	if err := s.policy.ValidateCodeExecution(*box, req.EnableNetwork); err != nil {
		return runner.Result{}, err
	}

	result, err := s.runner.RunInDir(ctx, runner.Request{
		Language:      req.Language,
		Code:          req.Code,
		Preload:       req.Preload,
		EnableNetwork: req.EnableNetwork,
	}, box.RootPath)
	if err != nil {
		return runner.Result{}, err
	}

	s.observer.Record("exec.code", req.SandboxID, "sandbox code executed", map[string]any{
		"language":  req.Language,
		"exit_code": result.ExitCode,
	})
	return result, nil
}

func (s *Service) RunCommand(ctx context.Context, req CommandRequest) (runner.CommandResult, error) {
	box, err := s.lifecycle.GetActive(req.SandboxID)
	if err != nil {
		return runner.CommandResult{}, err
	}

	workDir := box.RootPath
	if req.WorkingSubpath != "" {
		workDir, err = resolveSandboxPath(box.RootPath, req.WorkingSubpath)
		if err != nil {
			return runner.CommandResult{}, err
		}
	}

	timeout := s.policy.NormalizeCommandTimeout(req.TimeoutSeconds)
	result, err := s.runner.ExecuteCommand(ctx, workDir, req.Command, req.Args, timeout)
	if err != nil {
		return runner.CommandResult{}, err
	}

	s.observer.Record("exec.command", req.SandboxID, "sandbox command executed", map[string]any{
		"command":   req.Command,
		"exit_code": result.ExitCode,
	})
	return result, nil
}

func (s *Service) UploadFile(req FileWriteRequest) (*FileInfo, error) {
	box, err := s.lifecycle.GetActive(req.SandboxID)
	if err != nil {
		return nil, err
	}

	target, err := resolveSandboxPath(box.RootPath, req.Path)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, err
	}

	content, err := decodeContent(req.Content, req.Encoding)
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > s.policy.MaxFileSizeBytes() {
		return nil, fmt.Errorf("file exceeds max size of %d bytes", s.policy.MaxFileSizeBytes())
	}
	if err := os.WriteFile(target, content, 0o644); err != nil {
		return nil, err
	}

	info, err := s.StatFile(req.SandboxID, req.Path)
	if err == nil {
		s.observer.Record("files.upload", req.SandboxID, "file uploaded", map[string]any{"path": req.Path, "size": info.Size})
	}
	return info, err
}

func (s *Service) DownloadFile(sandboxID string, relativePath string, encoding string) (*FileContent, error) {
	box, err := s.lifecycle.GetActive(sandboxID)
	if err != nil {
		return nil, err
	}

	target, err := resolveSandboxPath(box.RootPath, relativePath)
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(target)
	if err != nil {
		return nil, err
	}

	normalized := normalizeEncoding(encoding)
	if normalized == "base64" {
		content = []byte(base64.StdEncoding.EncodeToString(content))
	}

	s.observer.Record("files.download", sandboxID, "file downloaded", map[string]any{"path": relativePath})
	return &FileContent{
		SandboxID: sandboxID,
		Path:      relativePath,
		Content:   string(content),
		Encoding:  normalized,
	}, nil
}

func (s *Service) StatFile(sandboxID string, relativePath string) (*FileInfo, error) {
	box, err := s.lifecycle.GetActive(sandboxID)
	if err != nil {
		return nil, err
	}

	target, err := resolveSandboxPath(box.RootPath, relativePath)
	if err != nil {
		return nil, err
	}

	stat, err := os.Stat(target)
	if err != nil {
		return nil, err
	}

	return &FileInfo{
		Path:        relativePath,
		Size:        stat.Size(),
		Mode:        stat.Mode().String(),
		ModifiedAt:  stat.ModTime().UTC(),
		IsDirectory: stat.IsDir(),
		SandboxID:   sandboxID,
	}, nil
}

func (s *Service) DeleteFile(sandboxID string, relativePath string) error {
	box, err := s.lifecycle.GetActive(sandboxID)
	if err != nil {
		return err
	}

	target, err := resolveSandboxPath(box.RootPath, relativePath)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(target); err != nil {
		return err
	}

	s.observer.Record("files.delete", sandboxID, "file deleted", map[string]any{"path": relativePath})
	return nil
}

func (s *Service) ListFiles(sandboxID string) ([]FileInfo, error) {
	box, err := s.lifecycle.GetActive(sandboxID)
	if err != nil {
		return nil, err
	}

	entries, err := ListDirectory(box.RootPath)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		entries[i].SandboxID = sandboxID
	}
	return entries, nil
}

func resolveSandboxPath(root string, relativePath string) (string, error) {
	if strings.TrimSpace(relativePath) == "" {
		return "", errors.New("path is required")
	}

	clean := filepath.Clean("/" + relativePath)
	target := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel, "..") {
		return "", errors.New("path escapes sandbox root")
	}
	return target, nil
}

func decodeContent(content string, encoding string) ([]byte, error) {
	switch normalizeEncoding(encoding) {
	case "base64":
		return base64.StdEncoding.DecodeString(content)
	default:
		return []byte(content), nil
	}
}

func normalizeEncoding(encoding string) string {
	if strings.EqualFold(strings.TrimSpace(encoding), "base64") {
		return "base64"
	}
	return "utf-8"
}

func ListDirectory(root string) ([]FileInfo, error) {
	entries := make([]FileInfo, 0)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return statErr
		}
		entries = append(entries, FileInfo{
			Path:        strings.TrimPrefix(path, root+"/"),
			Size:        info.Size(),
			Mode:        info.Mode().String(),
			ModifiedAt:  info.ModTime().UTC(),
			IsDirectory: d.IsDir(),
		})
		return nil
	})
	return entries, err
}
