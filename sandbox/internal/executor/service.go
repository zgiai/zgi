package executor

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
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
	SandboxID        string          `json:"sandbox_id"`
	Language         string          `json:"language"`
	Code             string          `json:"code"`
	Preload          string          `json:"preload"`
	InputJSON        json.RawMessage `json:"input_json,omitempty"`
	Profile          string          `json:"profile,omitempty"`
	TimeoutSeconds   int             `json:"timeout_seconds,omitempty"`
	TimeoutMS        int             `json:"timeout_ms,omitempty"`
	StdoutLimitKB    int             `json:"stdout_limit_kb,omitempty"`
	StderrLimitKB    int             `json:"stderr_limit_kb,omitempty"`
	StrictResultJSON bool            `json:"strict_result_json,omitempty"`
	EnableNetwork    bool            `json:"enable_network"`
}

type CommandRequest struct {
	SandboxID      string            `json:"sandbox_id"`
	Command        string            `json:"command"`
	Args           []string          `json:"args"`
	Stdin          string            `json:"stdin"`
	Env            map[string]string `json:"env"`
	Profile        string            `json:"profile"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	TimeoutMS      int               `json:"timeout_ms"`
	StdoutLimitKB  int               `json:"stdout_limit_kb"`
	StderrLimitKB  int               `json:"stderr_limit_kb"`
	WorkingSubpath string            `json:"working_subpath"`
}

type FileWriteRequest struct {
	SandboxID string `json:"sandbox_id"`
	Path      string `json:"path"`
	Content   string `json:"content"`
	Encoding  string `json:"encoding"`
}

type ArchiveUploadRequest struct {
	SandboxID             string `json:"sandbox_id"`
	Path                  string `json:"path"`
	ArchiveBase64         string `json:"archive_base64"`
	Format                string `json:"format"`
	StripRoot             bool   `json:"strip_root"`
	ValidateSkillManifest bool   `json:"validate_skill_manifest"`
}

type ArchiveUploadResult struct {
	SandboxID     string                  `json:"sandbox_id"`
	Path          string                  `json:"path"`
	Files         []FileInfo              `json:"files"`
	FileCount     int                     `json:"file_count"`
	TotalSize     int64                   `json:"total_size"`
	SkillManifest *SkillExecutionManifest `json:"skill_manifest,omitempty"`
}

type SkillExecutionManifest struct {
	Entrypoint           string   `json:"entrypoint"`
	Language             string   `json:"language"`
	TimeoutMS            int      `json:"timeout_ms"`
	AllowedArtifactPaths []string `json:"allowed_artifact_paths"`
	MaxArtifactCount     int      `json:"max_artifact_count"`
	MaxArtifactBytes     int64    `json:"max_artifact_bytes"`
	ResultMode           string   `json:"result_mode"`
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

type FileManifest struct {
	SandboxID string             `json:"sandbox_id"`
	Path      string             `json:"path"`
	Items     []FileManifestItem `json:"items"`
	FileCount int                `json:"file_count"`
	TotalSize int64              `json:"total_size"`
	Truncated bool               `json:"truncated"`
}

type FileManifestItem struct {
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	SHA256      string    `json:"sha256"`
	ContentType string    `json:"content_type"`
	ModifiedAt  time.Time `json:"modified_at"`
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
	limits, err := s.policy.NormalizeCommandLimits(defaultString(req.Profile, "code-short"), req.TimeoutSeconds, req.TimeoutMS, req.StdoutLimitKB, req.StderrLimitKB)
	if err != nil {
		return runner.Result{}, err
	}
	if len(req.InputJSON) > limits.MaxStdinBytes {
		return runner.Result{}, fmt.Errorf("input_json exceeds max size of %d bytes", limits.MaxStdinBytes)
	}

	runReq := runner.Request{
		Language:      req.Language,
		Code:          req.Code,
		Preload:       req.Preload,
		Stdin:         string(req.InputJSON),
		EnableNetwork: req.EnableNetwork,
	}

	result, err := s.runCodeWithScope(ctx, req, runReq, limits)
	if err != nil {
		return runner.Result{}, err
	}
	if err := attachResultJSON(&result, req.StrictResultJSON); err != nil {
		return runner.Result{}, err
	}

	s.observer.Record("exec.code", req.SandboxID, "sandbox code executed", map[string]any{
		"language":  req.Language,
		"profile":   limits.Profile,
		"exit_code": result.ExitCode,
	})
	return result, nil
}

func (s *Service) runCodeWithScope(ctx context.Context, req CodeRequest, runReq runner.Request, limits policy.CommandLimits) (runner.Result, error) {
	if strings.TrimSpace(req.SandboxID) == "" {
		if req.EnableNetwork {
			return runner.Result{}, errors.New("network access is disabled for stateless code execution")
		}
		return s.runner.RunWithLimits(ctx, runReq, limits.Timeout, limits.StdoutLimitBytes, limits.StderrLimitBytes)
	}

	box, err := s.lifecycle.GetActive(req.SandboxID)
	if err != nil {
		return runner.Result{}, err
	}
	if err := s.policy.ValidateCodeExecution(*box, req.EnableNetwork); err != nil {
		return runner.Result{}, err
	}
	return s.runner.RunInDirWithLimits(ctx, runReq, box.RootPath, limits.Timeout, limits.StdoutLimitBytes, limits.StderrLimitBytes)
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

	limits, err := s.policy.NormalizeCommandLimits(req.Profile, req.TimeoutSeconds, req.TimeoutMS, req.StdoutLimitKB, req.StderrLimitKB)
	if err != nil {
		return runner.CommandResult{}, err
	}
	if len(req.Stdin) > limits.MaxStdinBytes {
		return runner.CommandResult{}, fmt.Errorf("stdin exceeds max size of %d bytes", limits.MaxStdinBytes)
	}
	env, err := normalizeCommandEnv(req.Env)
	if err != nil {
		return runner.CommandResult{}, err
	}

	result, err := s.runner.ExecuteCommandSpec(ctx, runner.CommandSpec{
		WorkDir:        workDir,
		Command:        req.Command,
		Args:           req.Args,
		Stdin:          req.Stdin,
		Env:            env,
		Timeout:        limits.Timeout,
		StdoutLimit:    limits.StdoutLimitBytes,
		StderrLimit:    limits.StderrLimitBytes,
		AllowShellForm: true,
	})
	if err != nil {
		return runner.CommandResult{}, err
	}

	s.observer.Record("exec.command", req.SandboxID, "sandbox command executed", map[string]any{
		"command":   req.Command,
		"profile":   limits.Profile,
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
	var skillManifest *SkillExecutionManifest
	if req.ValidateSkillManifest {
		skillManifest, err = validateSkillExecutionManifest(entries, s.policy.MaxFileSizeBytes()*256)
		if err != nil {
			return nil, err
		}
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
		SandboxID:     req.SandboxID,
		Path:          req.Path,
		Files:         files,
		FileCount:     len(files),
		TotalSize:     totalSize,
		SkillManifest: skillManifest,
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

func (s *Service) BuildFileManifest(sandboxID string, relativePath string) (*FileManifest, error) {
	box, err := s.lifecycle.GetActive(sandboxID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(relativePath) == "" {
		relativePath = "artifacts"
	}

	target, err := resolveExistingSandboxPath(box.RootPath, relativePath)
	if err != nil {
		return nil, err
	}

	items := make([]FileManifestItem, 0)
	var totalSize int64
	truncated := false
	err = filepath.WalkDir(target, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == target || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("manifest path contains symlink: %s", path)
		}
		if len(items) >= 100 {
			truncated = true
			return filepath.SkipAll
		}

		item, err := fileManifestItem(box.RootPath, path, info)
		if err != nil {
			return err
		}
		items = append(items, item)
		totalSize += item.Size
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.observer.Record("files.manifest", sandboxID, "file manifest generated", map[string]any{
		"path":       relativePath,
		"file_count": len(items),
		"total_size": totalSize,
		"truncated":  truncated,
	})
	return &FileManifest{
		SandboxID: sandboxID,
		Path:      relativePath,
		Items:     items,
		FileCount: len(items),
		TotalSize: totalSize,
		Truncated: truncated,
	}, nil
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

func attachResultJSON(result *runner.Result, strict bool) error {
	if result == nil || result.ExitCode != 0 {
		return nil
	}
	raw := strings.TrimSpace(result.Stdout)
	if raw == "" {
		if strict {
			return errors.New("strict_result_json requires stdout to contain JSON")
		}
		return nil
	}

	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		if strict {
			return fmt.Errorf("strict_result_json failed to parse stdout JSON: %w", err)
		}
		return nil
	}
	result.ResultJSON = decoded
	return nil
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
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

func fileManifestItem(root string, path string, info fs.FileInfo) (FileManifestItem, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return FileManifestItem{}, err
	}
	sum := sha256.Sum256(content)
	rel := strings.TrimPrefix(path, root+string(filepath.Separator))
	contentType := "application/octet-stream"
	if len(content) > 0 {
		sample := content
		if len(sample) > 512 {
			sample = sample[:512]
		}
		contentType = http.DetectContentType(sample)
	}
	return FileManifestItem{
		Path:        filepath.ToSlash(rel),
		Size:        info.Size(),
		SHA256:      hex.EncodeToString(sum[:]),
		ContentType: contentType,
		ModifiedAt:  info.ModTime().UTC(),
	}, nil
}

func normalizeCommandEnv(env map[string]string) (map[string]string, error) {
	if len(env) == 0 {
		return nil, nil
	}
	if len(env) > 32 {
		return nil, errors.New("env exceeds max entry count of 32")
	}

	normalized := make(map[string]string, len(env))
	for key, value := range env {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, errors.New("env key is required")
		}
		if len(key) > 64 {
			return nil, fmt.Errorf("env key exceeds max length: %s", key)
		}
		if len(value) > 4096 {
			return nil, fmt.Errorf("env value for %s exceeds max length of 4096 bytes", key)
		}
		if !validEnvKey(key) {
			return nil, fmt.Errorf("invalid env key: %s", key)
		}
		if dangerousEnvKey(key) {
			return nil, fmt.Errorf("env key is not allowed: %s", key)
		}
		normalized[key] = value
	}
	return normalized, nil
}

func validEnvKey(key string) bool {
	for i, char := range key {
		if char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char == '_' {
			continue
		}
		if i > 0 && char >= '0' && char <= '9' {
			continue
		}
		return false
	}
	return true
}

func dangerousEnvKey(key string) bool {
	upper := strings.ToUpper(key)
	switch upper {
	case "PATH", "HOME", "IFS", "SHELLOPTS", "BASH_ENV", "ENV", "PYTHONPATH", "NODE_OPTIONS":
		return true
	default:
		return strings.HasPrefix(upper, "LD_") || strings.HasPrefix(upper, "DYLD_")
	}
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

func validateSkillExecutionManifest(entries []archiveEntry, maxArtifactBytes int64) (*SkillExecutionManifest, error) {
	names := make(map[string]archiveEntry, len(entries))
	for _, entry := range entries {
		names[filepath.ToSlash(entry.name)] = entry
	}

	manifestEntry, ok := names["skill.manifest.json"]
	if !ok {
		return nil, errors.New("skill.manifest.json is required when validate_skill_manifest is true")
	}
	if manifestEntry.file.UncompressedSize64 > 64*1024 {
		return nil, errors.New("skill.manifest.json exceeds max size of 65536 bytes")
	}
	content, err := readZipFile(manifestEntry.file, 64*1024)
	if err != nil {
		return nil, err
	}

	var manifest SkillExecutionManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return nil, fmt.Errorf("invalid skill.manifest.json: %w", err)
	}
	if err := validateSkillManifestFields(&manifest, names, maxArtifactBytes); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func validateSkillManifestFields(manifest *SkillExecutionManifest, names map[string]archiveEntry, maxArtifactBytes int64) error {
	manifest.Entrypoint = filepath.ToSlash(strings.TrimSpace(manifest.Entrypoint))
	if manifest.Entrypoint == "" {
		return errors.New("skill manifest entrypoint is required")
	}
	if unsafeArchivePath(manifest.Entrypoint) {
		return fmt.Errorf("skill manifest entrypoint escapes package root: %s", manifest.Entrypoint)
	}
	if _, ok := names[manifest.Entrypoint]; !ok {
		return fmt.Errorf("skill manifest entrypoint is missing from package: %s", manifest.Entrypoint)
	}

	manifest.Language = normalizeSkillLanguage(manifest.Language)
	if manifest.Language == "" {
		return errors.New("skill manifest language must be python3 or nodejs")
	}
	if manifest.TimeoutMS <= 0 || manifest.TimeoutMS > 300000 {
		return errors.New("skill manifest timeout_ms must be between 1 and 300000")
	}
	if manifest.MaxArtifactCount <= 0 || manifest.MaxArtifactCount > 100 {
		return errors.New("skill manifest max_artifact_count must be between 1 and 100")
	}
	if manifest.MaxArtifactBytes <= 0 || manifest.MaxArtifactBytes > maxArtifactBytes {
		return fmt.Errorf("skill manifest max_artifact_bytes must be between 1 and %d", maxArtifactBytes)
	}

	if len(manifest.AllowedArtifactPaths) == 0 {
		return errors.New("skill manifest allowed_artifact_paths is required")
	}
	for i, rawPath := range manifest.AllowedArtifactPaths {
		path := filepath.ToSlash(strings.TrimSpace(rawPath))
		if path == "" {
			return errors.New("skill manifest allowed_artifact_paths contains an empty path")
		}
		if unsafeArchivePath(path) {
			return fmt.Errorf("skill manifest allowed artifact path escapes package root: %s", path)
		}
		if path != "artifacts" && !strings.HasPrefix(path, "artifacts/") {
			return fmt.Errorf("skill manifest allowed artifact path must be under artifacts: %s", path)
		}
		manifest.AllowedArtifactPaths[i] = path
	}

	manifest.ResultMode = strings.TrimSpace(manifest.ResultMode)
	switch manifest.ResultMode {
	case "stdout_json", "stdout_text", "artifacts", "mixed":
		return nil
	default:
		return errors.New("skill manifest result_mode must be stdout_json, stdout_text, artifacts, or mixed")
	}
}

func normalizeSkillLanguage(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "python", "python3":
		return "python3"
	case "node", "nodejs", "javascript":
		return "nodejs"
	default:
		return ""
	}
}

func unsafeArchivePath(path string) bool {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "" || strings.HasPrefix(path, "/") || filepath.IsAbs(path) {
		return true
	}
	for _, part := range strings.Split(path, "/") {
		if part == ".." {
			return true
		}
	}
	return filepath.Clean(path) == "."
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
