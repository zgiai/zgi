package runtimeagent

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Reserved, disposable state, outside retained language caches. The outer
// Worker exclusively owns the workspace; a reconstructed Runtime Agent has no
// surviving Jobs and may reap directories left by an interrupted agent.
const jobTemporaryDirectory = ".zgi-runtime-tmp"

type temporaryDisposition bool

const (
	discardTemporary temporaryDisposition = true
	retainTemporary  temporaryDisposition = false
)

func isJobTemporaryPath(name string) bool {
	return name == jobTemporaryDirectory || strings.HasPrefix(name, jobTemporaryDirectory+"/")
}

func ownedTemporaryName(name string) bool {
	if !strings.HasPrefix(name, "job-") || len(name) != 36 {
		return false
	}
	_, err := hex.DecodeString(name[4:])
	return err == nil
}

// Called under b.mu, after checking idempotency and before starting a process.
// Pin the parent with os.Root: removing a Job-created link never follows it
// into user files or the host, even if a Job replaces a directory concurrently.
func (b *Backend) prepareJobTemporaryDirectory() (string, func(temporaryDisposition) error, error) {
	if err := b.workspace.MkdirAll(jobTemporaryDirectory, 0o755); err != nil {
		return "", nil, err
	}
	info, err := b.workspace.Lstat(jobTemporaryDirectory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", nil, errors.New("temporary parent must be a directory, not a link")
	}
	root, err := b.workspace.OpenRoot(jobTemporaryDirectory)
	if err != nil {
		return "", nil, err
	}
	fail := func(err error) (string, func(temporaryDisposition) error, error) {
		return "", nil, errors.Join(err, root.Close())
	}
	// Recheck the opened identity; do not follow a raced parent substitution.
	file, err := root.Open(".")
	if err != nil {
		return fail(err)
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		_ = file.Close()
		return fail(errors.New("temporary parent changed"))
	}
	if !b.temporaryReady {
		entries, err := file.ReadDir(-1)
		if err != nil {
			_ = file.Close()
			return fail(err)
		}
		for _, entry := range entries {
			if ownedTemporaryName(entry.Name()) {
				if err := root.RemoveAll(entry.Name()); err != nil {
					_ = file.Close()
					return fail(fmt.Errorf("reap interrupted job temporary data: %w", err))
				}
			}
		}
		b.temporaryReady = true
	}
	if err := file.Close(); err != nil {
		return fail(err)
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return fail(err)
	}
	name := "job-" + hex.EncodeToString(nonce[:])
	if err := root.Mkdir(name, 0o700); err != nil {
		return fail(err)
	}
	cleanup := func(disposition temporaryDisposition) error {
		if disposition == retainTemporary {
			return root.Close()
		}
		return errors.Join(root.RemoveAll(name), root.Close())
	}
	directory, err := root.Open(name)
	if err != nil {
		return "", nil, errors.Join(err, cleanup(discardTemporary))
	}
	if err := errors.Join(prepareJobFileOwnership(directory, b.jobUID, b.jobGID), directory.Close()); err != nil {
		return "", nil, errors.Join(err, cleanup(discardTemporary))
	}
	return filepath.Join(b.workspaceName, jobTemporaryDirectory, name), cleanup, nil
}
