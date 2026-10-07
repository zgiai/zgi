package runtimeagent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// cacheNamespace keeps caches within the session's tenant-owned workspace.
// Profile/ABI/source changes do not reuse prior downloaded or compiled content.
func (b *Backend) cacheNamespace(ctx context.Context) (string, error) {
	if b.profileIdentity == "" {
		return "", nil
	} // Legacy direct callers.
	b.cacheMu.Lock()
	defer b.cacheMu.Unlock()
	if b.cacheKey != "" {
		return b.cacheKey, nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	python, err := exec.CommandContext(probeCtx, "python3", "-I", "-c", "import sys,platform; print(sys.implementation.cache_tag+':'+platform.machine())").Output()
	if err != nil {
		return "", fmt.Errorf("identify Python cache ABI: %w", err)
	}
	node, err := exec.CommandContext(probeCtx, "node", "--no-addons", "-p", "process.versions.modules+':'+process.arch").Output()
	if err != nil {
		return "", fmt.Errorf("identify Node cache ABI: %w", err)
	}
	identity := struct{ Profile, Python, Node string }{b.profileIdentity, strings.TrimSpace(string(python)), strings.TrimSpace(string(node))}
	data, err := json.Marshal(identity)
	if err != nil {
		return "", err
	}
	marker := filepath.Join(sessionHomeDirectory, "runtime-profile.json")
	file, err := b.workspace.Open(marker)
	if err == nil {
		previous, readErr := io.ReadAll(io.LimitReader(file, 4097))
		err = errors.Join(readErr, file.Close())
		if err != nil {
			return "", err
		}
		if string(previous) != string(data) {
			return "", errors.New("retained dependencies use a different runtime profile or ABI; recover with the original profile")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	} else {
		// The marker is unprivileged workspace data, never a root build input.
		if err := b.workspace.WriteFile(marker, data, 0o600); err != nil {
			return "", err
		}
	}
	digest := sha256.Sum256(append(data, []byte(b.packageSources.JSON())...))
	b.cacheKey = hex.EncodeToString(digest[:])
	return b.cacheKey, nil
}
