// Package workspacecheckpoint stores immutable Agent workspace archives on a
// durable worker volume. It does not provide replication across worker nodes.
package workspacecheckpoint

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"regexp"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

const maxCheckpointBytes int64 = (1 << 30) + (40 << 20)

var safeKey = regexp.MustCompile(`^[a-f0-9]{64}$`)

type FileStore struct {
	root *os.Root
}

type manifest struct {
	Version int    `json:"version"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
}

var _ agentexecution.WorkspaceCheckpointStore = (*FileStore)(nil)

func NewFileStore(directory string) (*FileStore, error) {
	if directory == "" {
		return nil, fmt.Errorf("%w: checkpoint directory is required", agentexecution.ErrInvalidRequest)
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("prepare checkpoint directory: %w", err)
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: checkpoint directory must be a real directory", agentexecution.ErrInvalidRequest)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	return &FileStore{root: root}, nil
}

func (s *FileStore) Close() error { return s.root.Close() }

func (s *FileStore) Save(ctx context.Context, key string, source io.Reader) error {
	if !safeKey.MatchString(key) || source == nil {
		return agentexecution.ErrInvalidRequest
	}
	unlock, err := s.lockKey(ctx, key)
	if err != nil {
		return err
	}
	defer unlock()
	if _, err := s.root.Stat(key + ".json"); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary, err := randomTemporaryName()
	if err != nil {
		return err
	}
	defer s.root.Remove(temporary)
	file, err := s.root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	digest := sha256.New()
	count, copyErr := io.Copy(io.MultiWriter(file, digest), io.LimitReader(&contextReader{ctx: ctx, reader: source}, maxCheckpointBytes+1))
	if count > maxCheckpointBytes {
		copyErr = agentexecution.ErrOutputLimit
	}
	if copyErr == nil {
		copyErr = file.Sync()
	}
	closeErr := file.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return fmt.Errorf("write workspace checkpoint: %w", err)
	}
	if err := s.root.Rename(temporary, key+".tar"); err != nil {
		return err
	}
	metadata := manifest{Version: 1, Size: count, SHA256: hex.EncodeToString(digest.Sum(nil))}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	metadataTemporary, err := randomTemporaryName()
	if err != nil {
		return err
	}
	defer s.root.Remove(metadataTemporary)
	metadataFile, err := s.root.OpenFile(metadataTemporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := metadataFile.Write(encoded)
	if writeErr == nil {
		writeErr = metadataFile.Sync()
	}
	closeErr = metadataFile.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	if err := s.root.Rename(metadataTemporary, key+".json"); err != nil {
		return err
	}
	return syncDirectory(s.root.Name())
}

func (s *FileStore) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if !safeKey.MatchString(key) {
		return nil, agentexecution.ErrInvalidRequest
	}
	value, err := s.root.ReadFile(key + ".json")
	if errors.Is(err, os.ErrNotExist) {
		return nil, agentexecution.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var metadata manifest
	if err := json.Unmarshal(value, &metadata); err != nil || metadata.Version != 1 || metadata.Size < 0 || metadata.Size > maxCheckpointBytes || !safeKey.MatchString(metadata.SHA256) {
		return nil, fmt.Errorf("%w: invalid checkpoint metadata", agentexecution.ErrBackendUnavailable)
	}
	file, err := s.root.Open(key + ".tar")
	if err != nil {
		return nil, fmt.Errorf("%w: checkpoint archive is unavailable", agentexecution.ErrBackendUnavailable)
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != metadata.Size {
		_ = file.Close()
		return nil, fmt.Errorf("%w: checkpoint archive size changed", agentexecution.ErrBackendUnavailable)
	}
	return &verifyingReader{ctx: ctx, file: file, remaining: metadata.Size, expected: metadata.SHA256, hash: sha256.New()}, nil
}

func (s *FileStore) Delete(ctx context.Context, key string) error {
	if !safeKey.MatchString(key) {
		return agentexecution.ErrInvalidRequest
	}
	unlock, err := s.lockKey(ctx, key)
	if err != nil {
		return err
	}
	defer unlock()
	for _, suffix := range []string{".json", ".tar"} {
		if err := s.root.Remove(key + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return syncDirectory(s.root.Name())
}

type verifyingReader struct {
	ctx       context.Context
	file      *os.File
	remaining int64
	expected  string
	hash      hash.Hash
}

func (r *verifyingReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.file.Read(buffer)
	if n > 0 {
		r.remaining -= int64(n)
		_, _ = r.hash.Write(buffer[:n])
	}
	if errors.Is(err, io.EOF) {
		if r.remaining != 0 || hex.EncodeToString(r.hash.Sum(nil)) != r.expected {
			return n, fmt.Errorf("%w: workspace checkpoint checksum mismatch", agentexecution.ErrBackendUnavailable)
		}
	}
	return n, err
}

func (r *verifyingReader) Close() error { return r.file.Close() }

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}

func randomTemporaryName() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return ".checkpoint-" + hex.EncodeToString(value[:]) + ".tmp", nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}
