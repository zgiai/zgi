package executor

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
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

type ArchiveUploadRequest struct {
	SandboxID     string `json:"sandbox_id"`
	Path          string `json:"path"`
	ArchiveBase64 string `json:"archive_base64"`
	Format        string `json:"format"`
	StripRoot     bool   `json:"strip_root"`
}

type ArchiveUploadResult struct {
	SandboxID string     `json:"sandbox_id"`
	Path      string     `json:"path"`
	Files     []FileInfo `json:"files"`
	FileCount int        `json:"file_count"`
	TotalSize int64      `json:"total_size"`
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
		workDir, err = resolveExistingSandboxPath(box.RootPath, req.WorkingSubpath)
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

	target, err := resolveWritableSandboxPath(box.RootPath, req.Path)
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

func (s *Service) UploadArchive(req ArchiveUploadRequest) (*ArchiveUploadResult, error) {
	if strings.TrimSpace(req.SandboxID) == "" {
		return nil, errors.New("sandbox_id is required")
	}
	if strings.TrimSpace(req.Path) == "" {
		req.Path = "."
	}
	if strings.TrimSpace(req.ArchiveBase64) == "" {
		return nil, errors.New("archive_base64 is required")
	}
	if !strings.EqualFold(strings.TrimSpace(req.Format), "zip") {
		return nil, errors.New("unsupported archive format")
	}

	box, err := s.lifecycle.GetActive(req.SandboxID)
	if err != nil {
		return nil, err
	}
	if _, err := resolveWritableSandboxPath(box.RootPath, req.Path); err != nil {
		return nil, err
	}

	archiveBytes, err := base64.StdEncoding.DecodeString(req.ArchiveBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid archive_base64: %w", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
	if err != nil {
		return nil, fmt.Errorf("invalid zip archive: %w", err)
	}

	entries, err := normalizeArchiveEntries(reader.File, req.StripRoot)
	if err != nil {
		return nil, err
	}

	limit := archiveLimits{
		maxFiles:     256,
		maxFileSize:  s.policy.MaxFileSizeBytes(),
		maxTotalSize: s.policy.MaxFileSizeBytes() * 256,
	}

	written := make([]fileSnapshot, 0, len(entries))
	files := make([]FileInfo, 0, len(entries))
	var totalSize int64
	for _, entry := range entries {
		if len(files) >= limit.maxFiles {
			rollbackWrittenFiles(written)
			return nil, fmt.Errorf("archive exceeds max file count of %d", limit.maxFiles)
		}
		if entry.file.FileInfo().Mode()&os.ModeSymlink != 0 {
			rollbackWrittenFiles(written)
			return nil, fmt.Errorf("archive contains symlink: %s", entry.name)
		}
		if entry.file.UncompressedSize64 > uint64(limit.maxFileSize) {
			rollbackWrittenFiles(written)
			return nil, fmt.Errorf("file %s exceeds max size of %d bytes", entry.name, limit.maxFileSize)
		}
		totalSize += int64(entry.file.UncompressedSize64)
		if totalSize > limit.maxTotalSize {
			rollbackWrittenFiles(written)
			return nil, fmt.Errorf("archive exceeds max total size of %d bytes", limit.maxTotalSize)
		}

		relativePath := filepath.ToSlash(filepath.Join(req.Path, entry.name))
		target, err := resolveWritableSandboxPath(box.RootPath, relativePath)
		if err != nil {
			rollbackWrittenFiles(written)
			return nil, err
		}
		snapshot, err := snapshotExistingFile(target)
		if err != nil {
			rollbackWrittenFiles(written)
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			rollbackWrittenFiles(written)
			return nil, err
		}

		content, err := readZipFile(entry.file, limit.maxFileSize)
		if err != nil {
			rollbackWrittenFiles(written)
			return nil, err
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			rollbackWrittenFiles(written)
			return nil, err
		}
		written = append(written, snapshot)

		info, err := s.StatFile(req.SandboxID, relativePath)
		if err != nil {
			rollbackWrittenFiles(written)
			return nil, err
		}
		files = append(files, *info)
	}

	s.observer.Record("files.upload_archive", req.SandboxID, "archive uploaded", map[string]any{
		"path":       req.Path,
		"file_count": len(files),
		"total_size": totalSize,
	})
	return &ArchiveUploadResult{
		SandboxID: req.SandboxID,
		Path:      req.Path,
		Files:     files,
		FileCount: len(files),
		TotalSize: totalSize,
	}, nil
}

func (s *Service) DownloadFile(sandboxID string, relativePath string, encoding string) (*FileContent, error) {
	box, err := s.lifecycle.GetActive(sandboxID)
	if err != nil {
		return nil, err
	}

	target, err := resolveExistingSandboxPath(box.RootPath, relativePath)
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

	target, err := resolveExistingSandboxPath(box.RootPath, relativePath)
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
	if filepath.Clean("/"+relativePath) == "/" {
		return errors.New("cannot delete sandbox root")
	}

	target, err := resolveExistingSandboxPath(box.RootPath, relativePath)
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

	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	clean := filepath.Clean("/" + relativePath)
	target := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes sandbox root")
	}
	return target, nil
}

func resolveExistingSandboxPath(root string, relativePath string) (string, error) {
	target, err := resolveSandboxPath(root, relativePath)
	if err != nil {
		return "", err
	}
	if err := rejectSymlinkPath(root, target, false); err != nil {
		return "", err
	}
	return target, nil
}

func resolveWritableSandboxPath(root string, relativePath string) (string, error) {
	target, err := resolveSandboxPath(root, relativePath)
	if err != nil {
		return "", err
	}
	if err := rejectSymlinkPath(root, target, true); err != nil {
		return "", err
	}
	return target, nil
}

func rejectSymlinkPath(root string, target string, allowMissingTail bool) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return err
	}

	rootInfo, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("sandbox root is a symlink")
	}

	rel, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("path escapes sandbox root")
	}

	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) && allowMissingTail {
				return nil
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path contains symlink: %s", current)
		}
	}
	return nil
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

type archiveLimits struct {
	maxFiles     int
	maxFileSize  int64
	maxTotalSize int64
}

type archiveEntry struct {
	name string
	file *zip.File
}

func normalizeArchiveEntries(files []*zip.File, stripRoot bool) ([]archiveEntry, error) {
	root := commonArchiveRoot(files)
	entries := make([]archiveEntry, 0, len(files))
	for _, file := range files {
		name := strings.TrimSpace(filepath.ToSlash(file.Name))
		if name == "" {
			continue
		}
		if strings.HasPrefix(name, "/") || strings.Contains(name, "\x00") {
			return nil, fmt.Errorf("invalid archive path: %s", file.Name)
		}
		if stripRoot && root != "" {
			name = strings.TrimPrefix(name, root+"/")
		}
		name = strings.TrimPrefix(name, "./")
		if name == "" || strings.HasSuffix(name, "/") || file.FileInfo().IsDir() {
			continue
		}
		clean := filepath.ToSlash(filepath.Clean(name))
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, fmt.Errorf("archive path escapes target root: %s", file.Name)
		}
		entries = append(entries, archiveEntry{name: clean, file: file})
	}
	return entries, nil
}

func commonArchiveRoot(files []*zip.File) string {
	root := ""
	for _, file := range files {
		name := strings.Trim(filepath.ToSlash(file.Name), "/")
		if name == "" {
			continue
		}
		if file.FileInfo().IsDir() && !strings.Contains(name, "/") {
			continue
		}
		parts := strings.Split(name, "/")
		if len(parts) < 2 {
			return ""
		}
		if root == "" {
			root = parts[0]
			continue
		}
		if root != parts[0] {
			return ""
		}
	}
	return root
}

func readZipFile(file *zip.File, maxSize int64) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	content := make([]byte, 0)
	buffer := make([]byte, 32*1024)
	var total int64
	for {
		n, readErr := reader.Read(buffer)
		if n > 0 {
			total += int64(n)
			if total > maxSize {
				return nil, fmt.Errorf("file %s exceeds max size of %d bytes", file.Name, maxSize)
			}
			content = append(content, buffer[:n]...)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	return content, nil
}

type fileSnapshot struct {
	path    string
	exists  bool
	content []byte
	mode    os.FileMode
}

func snapshotExistingFile(path string) (fileSnapshot, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileSnapshot{path: path}, nil
		}
		return fileSnapshot{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fileSnapshot{}, fmt.Errorf("target path is a symlink: %s", path)
	}
	if info.IsDir() {
		return fileSnapshot{}, fmt.Errorf("target path is a directory: %s", path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return fileSnapshot{}, err
	}
	return fileSnapshot{
		path:    path,
		exists:  true,
		content: content,
		mode:    info.Mode().Perm(),
	}, nil
}

func rollbackWrittenFiles(files []fileSnapshot) {
	for i := len(files) - 1; i >= 0; i-- {
		file := files[i]
		if file.exists {
			_ = os.WriteFile(file.path, file.content, file.mode)
			continue
		}
		_ = os.Remove(file.path)
	}
}
