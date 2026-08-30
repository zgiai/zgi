package contextmgr

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var safePathPartPattern = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// FileStore persists raw oversized tool results below the runtime storage
// directory. The files are runtime data and must remain out of source control.
type FileStore struct {
	root string
}

func NewFileStore(root string) *FileStore {
	return &FileStore{root: filepath.Clean(root)}
}

func (s *FileStore) Put(ctx context.Context, agentRunID string, contentHash string, content string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	path, runPart, hashPart, err := s.toolResultPath(agentRunID, contentHash)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err == nil {
		return "agent-context://tool-results/" + runPart + "/" + hashPart, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("stat tool result: %w", err)
	}
	if err := atomicWrite(path, []byte(content), 0o600); err != nil {
		return "", err
	}
	return "agent-context://tool-results/" + runPart + "/" + hashPart, nil
}

func (s *FileStore) Get(ctx context.Context, agentRunID string, contentHash string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	path, _, _, err := s.toolResultPath(agentRunID, contentHash)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("context artifact not found")
	}
	if err != nil {
		return "", fmt.Errorf("read context artifact: %w", err)
	}
	return string(content), nil
}

func (s *FileStore) toolResultPath(agentRunID string, contentHash string) (string, string, string, error) {
	if s == nil || strings.TrimSpace(s.root) == "" || s.root == "." {
		return "", "", "", fmt.Errorf("agent context storage root is required")
	}
	runPart, err := safePathPart(agentRunID)
	if err != nil {
		return "", "", "", err
	}
	hashPart, err := safePathPart(contentHash)
	if err != nil {
		return "", "", "", err
	}
	return filepath.Join(s.root, "tool-results", runPart, hashPart+".txt"), runPart, hashPart, nil
}

func safePathPart(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("storage key is required")
	}
	safe := strings.Trim(safePathPartPattern.ReplaceAllString(value, "_"), "._-")
	if safe == "" {
		return "", fmt.Errorf("storage key is invalid")
	}
	return safe, nil
}

func atomicWrite(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create storage directory: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".context-*")
	if err != nil {
		return fmt.Errorf("create temporary context file: %w", err)
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set context file mode: %w", err)
	}
	if _, err := temp.Write(content); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write context file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync context file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close context file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace context file: %w", err)
	}
	return nil
}
