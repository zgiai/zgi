package runtimeagent

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

const (
	maxWorkspaceArchiveBytes int64 = 1 << 30
	maxWorkspaceEntries            = 10000
)

var _ agentexecution.WorkspaceArchiver = (*Backend)(nil)

// ExportWorkspace emits only directories and regular files. A workspace must
// be quiescent before its caller captures a checkpoint; this method does not
// freeze running processes on its own.
func (b *Backend) ExportWorkspace(ctx context.Context, _ agentexecution.Allocation) (io.ReadCloser, error) {
	reader, writer := io.Pipe()
	go func() {
		archive := tar.NewWriter(writer)
		err := b.writeWorkspaceArchive(ctx, archive)
		if closeErr := archive.Close(); err == nil {
			err = closeErr
		}
		_ = writer.CloseWithError(err)
	}()
	return reader, nil
}

func (b *Backend) writeWorkspaceArchive(ctx context.Context, archive *tar.Writer) error {
	var total int64
	entries := 0
	return fs.WalkDir(b.workspace.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		entries++
		if entries > maxWorkspaceEntries {
			return agentexecution.ErrOutputLimit
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header := &tar.Header{Name: name, Mode: int64(info.Mode().Perm())}
		if entry.IsDir() {
			header.Typeflag = tar.TypeDir
			return archive.WriteHeader(header)
		}
		if !entry.Type().IsRegular() || !info.Mode().IsRegular() {
			return fmt.Errorf("%w: checkpoint contains a non-regular file", agentexecution.ErrInvalidPath)
		}
		if info.Size() < 0 || info.Size() > maxWorkspaceArchiveBytes-total {
			return agentexecution.ErrOutputLimit
		}
		total += info.Size()
		header.Typeflag = tar.TypeReg
		header.Size = info.Size()
		if err := archive.WriteHeader(header); err != nil {
			return err
		}
		file, err := b.workspace.Open(name)
		if err != nil {
			return err
		}
		openedInfo, err := file.Stat()
		if err != nil || !openedInfo.Mode().IsRegular() || openedInfo.Size() != info.Size() {
			_ = file.Close()
			return fmt.Errorf("%w: workspace changed during checkpoint", agentexecution.ErrInvalidTransition)
		}
		_, copyErr := io.CopyN(archive, file, info.Size())
		return errors.Join(copyErr, file.Close())
	})
}

// ImportWorkspace accepts a checkpoint only into an empty workspace. The
// caller must destroy a newly created environment if import fails, because a
// failed stream can leave a partial tree behind.
func (b *Backend) ImportWorkspace(ctx context.Context, _ agentexecution.Allocation, source io.Reader) error {
	if source == nil {
		return agentexecution.ErrInvalidRequest
	}
	entries, err := fs.ReadDir(b.workspace.FS(), ".")
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("%w: workspace is not empty", agentexecution.ErrInvalidTransition)
	}
	const maximum = maxWorkspaceArchiveBytes + int64(maxWorkspaceEntries)*4096
	limited := &io.LimitedReader{R: source, N: maximum + 1}
	archive := tar.NewReader(limited)
	seen := make(map[string]struct{})
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			// Drain the transport so the remote trailer and durable-store digest
			// are both checked before reporting a successful restore.
			if _, err := io.Copy(io.Discard, limited); err != nil {
				return err
			}
			if limited.N == 0 {
				return agentexecution.ErrOutputLimit
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("read workspace archive: %w", err)
		}
		name := strings.TrimSuffix(header.Name, "/")
		clean, err := cleanRelativePath(name, false)
		if err != nil || clean != name || len(name) > 4096 {
			return agentexecution.ErrInvalidPath
		}
		if _, ok := seen[clean]; ok {
			return fmt.Errorf("%w: duplicate checkpoint path", agentexecution.ErrInvalidPath)
		}
		seen[clean] = struct{}{}
		if len(seen) > maxWorkspaceEntries {
			return agentexecution.ErrOutputLimit
		}
		mode := fs.FileMode(header.Mode) & 0o750
		switch header.Typeflag {
		case tar.TypeDir:
			if err := b.workspace.MkdirAll(clean, mode|0o700); err != nil {
				return fmt.Errorf("restore workspace directory: %w", err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || header.Size > maxWorkspaceArchiveBytes-total {
				return agentexecution.ErrOutputLimit
			}
			total += header.Size
			if parent := path.Dir(clean); parent != "." {
				if err := b.workspace.MkdirAll(parent, 0o750); err != nil {
					return fmt.Errorf("restore workspace parent: %w", err)
				}
			}
			file, err := b.workspace.OpenFile(clean, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode|0o600)
			if err != nil {
				return fmt.Errorf("create restored file: %w", err)
			}
			written, copyErr := io.CopyN(file, archive, header.Size)
			closeErr := file.Close()
			if copyErr != nil || closeErr != nil || written != header.Size {
				return errors.Join(copyErr, closeErr, fmt.Errorf("restore workspace file: incomplete write"))
			}
		default:
			return fmt.Errorf("%w: checkpoint contains an unsupported entry", agentexecution.ErrInvalidPath)
		}
	}
}
