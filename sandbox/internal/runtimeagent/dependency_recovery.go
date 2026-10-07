package runtimeagent

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

type dependencyContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r dependencyContextReader) Read(value []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(value)
}

// The open descriptor, not a later lookup of its Job-controlled name, is used
// for both verification and decompression. Nothing in the archive is executed.
func (b *Backend) openDependencyArtifact(ctx context.Context, artifact dependencyArtifact) (*os.File, error) {
	info, err := b.workspace.Lstat(artifact.name())
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() != artifact.Size {
		return nil, errors.New("dependency artifact size or type mismatch")
	}
	file, err := openRegularWorkspaceFile(b.workspace, artifact.name())
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = file.Close()
		}
	}()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, errors.New("dependency artifact changed during open")
	}
	hash := sha256.New()
	n, err := io.Copy(hash, dependencyContextReader{ctx: ctx, reader: io.LimitReader(file, artifact.Size+1)})
	if err != nil {
		return nil, err
	}
	if n != artifact.Size || hex.EncodeToString(hash.Sum(nil)) != artifact.SHA256 {
		return nil, errors.New("dependency artifact checksum mismatch")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	keep = true
	return file, nil
}

type stagedDependency struct {
	artifact dependencyArtifact
	stage    string
}

// Names are derived from ledger-owned targets, rather than random paths that
// become untraceable if the agent dies between the two directory renames.
func dependencyRecoveryNames(artifact dependencyArtifact) (stage, backup string) {
	digest := sha256.Sum256([]byte(artifact.Target + "\x00" + artifact.SHA256))
	prefix := dependencyArtifactDirectory + "/restore-" + hex.EncodeToString(digest[:])
	return prefix + ".stage", prefix + ".backup"
}

func (b *Backend) recoverInterruptedDependency(artifact dependencyArtifact) error {
	stage, backup := dependencyRecoveryNames(artifact)
	info, err := b.workspace.Lstat(backup)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("dependency recovery backup is not a directory")
		}
		target, err := b.workspace.Lstat(artifact.Target)
		switch {
		case errors.Is(err, os.ErrNotExist):
			if err := b.workspace.Rename(backup, artifact.Target); err != nil {
				return err
			}
		case err != nil:
			return err
		case !target.IsDir() || target.Mode()&os.ModeSymlink != 0:
			return errors.New("dependency recovery target is not a directory")
		default:
			if err := b.workspace.RemoveAll(backup); err != nil {
				return err
			}
		}
	}
	info, err = b.workspace.Lstat(stage)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("dependency recovery stage is not a directory")
	}
	return b.workspace.RemoveAll(stage)
}

func (b *Backend) stageDependencyArtifact(ctx context.Context, artifact dependencyArtifact) (stagedDependency, error) {
	stage, _ := dependencyRecoveryNames(artifact)
	result := stagedDependency{artifact: artifact, stage: stage}
	file, err := b.openDependencyArtifact(ctx, artifact)
	if err != nil {
		return result, err
	}
	defer file.Close()
	compressed, err := gzip.NewReader(dependencyContextReader{ctx: ctx, reader: file})
	if err != nil {
		return result, err
	}
	defer compressed.Close()
	if err := b.workspace.Mkdir(result.stage, 0o700); err != nil {
		return result, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = b.workspace.RemoveAll(result.stage)
		}
	}()
	root, err := b.workspace.OpenRoot(result.stage)
	if err != nil {
		return result, err
	}
	defer root.Close()
	// The private staging root stays root-owned until the completed directory
	// is swapped in. Imported children get the ordinary non-root Job owner.
	importer := Backend{workspace: root, jobUID: b.jobUID, jobGID: b.jobGID}
	if err := importer.ImportWorkspace(ctx, agentexecution.Allocation{}, compressed); err != nil {
		return result, err
	}
	actual, err := encodeDependencyArchive(ctx, root, artifact.Target, io.Discard)
	if err != nil {
		return result, err
	}
	if actual != artifact {
		return result, errors.New("dependency archive is not a canonical installation artifact")
	}
	if err := prepareJobDirectoryOwnership(root, b.jobUID, b.jobGID); err != nil {
		return result, err
	}
	keep = true
	return result, nil
}

func prepareJobDirectoryOwnership(root *os.Root, uid, gid uint32) error {
	file, err := root.Open(".")
	if err != nil {
		return err
	}
	return errors.Join(prepareJobFileOwnership(file, uid, gid), file.Close())
}

func (b *Backend) commitDependencyArtifact(staged stagedDependency) error {
	target := staged.artifact.Target
	info, err := b.workspace.Lstat(target)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if exists && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return errors.New("dependency recovery target is not a directory")
	}
	if err := b.workspace.MkdirAll(path.Dir(target), 0o700); err != nil {
		return err
	}
	_, backup := dependencyRecoveryNames(staged.artifact)
	if exists {
		if err := b.workspace.Rename(target, backup); err != nil {
			return err
		}
	}
	if err := b.workspace.Rename(staged.stage, target); err != nil {
		if exists {
			return errors.Join(err, b.workspace.Rename(backup, target))
		}
		return err
	}
	if exists {
		return b.workspace.RemoveAll(backup)
	}
	return nil
}

func (b *Backend) restoreDependencyArtifacts(ctx context.Context, artifacts []dependencyArtifact) error {
	var staged []stagedDependency
	defer func() {
		for _, value := range staged {
			_ = b.workspace.RemoveAll(value.stage)
		}
	}()
	for _, artifact := range artifacts {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := b.recoverInterruptedDependency(artifact); err != nil {
			return err
		}
		actual, err := b.dependencyArchive(ctx, artifact.Target, io.Discard)
		if err == nil && actual == artifact {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		value, err := b.stageDependencyArtifact(ctx, artifact)
		if err != nil {
			return err
		}
		staged = append(staged, value)
	}
	// Validate all required artifacts before changing any live installation.
	for _, value := range staged {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := b.commitDependencyArtifact(value); err != nil {
			return err
		}
		actual, err := b.dependencyArchive(ctx, value.artifact.Target, io.Discard)
		if err != nil {
			return err
		}
		if actual != value.artifact {
			return errors.New("restored dependency artifact fingerprint mismatch")
		}
	}
	return nil
}
