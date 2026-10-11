package runtimeagent

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

const dependencyArtifactDirectory = sessionHomeDirectory + "/.dependency-artifacts"
const maxDependencyArtifactBytes int64 = 256 << 20
const maxDependencyArtifactTotal int64 = 512 << 20
const maxDependencyArtifacts = 64

var lowercaseDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)

// An installed-state artifact contains compiled files, not an upstream wheel or
// registry tarball. It is session-owned and never executes an installer as root.
type dependencyArtifact struct {
	Target string `json:"target"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

func (a dependencyArtifact) name() string {
	return dependencyArtifactDirectory + "/" + a.SHA256 + ".tar.gz"
}

func dependencyTargets(packages []languagePackage) []string {
	seen := make(map[string]bool)
	for _, pkg := range packages {
		if pkg.Language == "python" {
			seen[sessionHomeDirectory+"/.venv"] = true
			continue
		}
		parts := strings.Split(pkg.Path, "/")
		for index, part := range parts {
			if part == "node_modules" {
				seen[strings.Join(parts[:index+1], "/")] = true
				break
			}
		}
	}
	var targets []string
	for target := range seen {
		targets = append(targets, target)
	}
	slices.Sort(targets)
	return slices.DeleteFunc(targets, func(target string) bool {
		for parent := path.Dir(target); parent != "."; parent = path.Dir(parent) {
			if seen[parent] {
				return true
			}
		}
		return false
	})
}

func validDependencyTarget(target string) bool {
	clean, err := cleanRelativePath(target, false)
	if err != nil || clean != target || len(target) > 4096 {
		return false
	}
	return target == sessionHomeDirectory+"/.venv" || path.Base(target) == "node_modules" && !strings.Contains("/"+path.Dir(target)+"/", "/node_modules/") && !strings.HasPrefix(target, dependencyArtifactDirectory+"/")
}

func (b *Backend) validateDependencyArtifacts(ledger languagePackageLedger) error {
	if len(ledger.Artifacts)+len(ledger.UncachedTargets) > maxDependencyArtifacts {
		return agentexecution.ErrOutputLimit
	}
	targets := dependencyTargets(ledger.Packages)
	if len(targets) != len(ledger.Artifacts)+len(ledger.UncachedTargets) {
		return errors.New("dependency artifacts do not cover the installed inventory")
	}
	seen := make(map[string]bool)
	var total int64
	for _, artifact := range ledger.Artifacts {
		decoded, err := hex.DecodeString(artifact.SHA256)
		if !validDependencyTarget(artifact.Target) || seen[artifact.Target] || !slices.Contains(targets, artifact.Target) || err != nil || len(decoded) != sha256.Size || hex.EncodeToString(decoded) != artifact.SHA256 || artifact.Size <= 0 || artifact.Size > maxDependencyArtifactBytes {
			return errors.New("invalid or duplicate dependency artifact")
		}
		seen[artifact.Target] = true
		total += artifact.Size
		if total > maxDependencyArtifactTotal {
			return agentexecution.ErrOutputLimit
		}
	}
	for _, target := range ledger.UncachedTargets {
		if !validDependencyTarget(target) || seen[target] || !slices.Contains(targets, target) {
			return errors.New("invalid or duplicate uncached dependency target")
		}
		seen[target] = true
	}
	return nil
}

type artifactWriter struct {
	writer    io.Writer
	remaining int64
	written   int64
}

func (w *artifactWriter) Write(value []byte) (int, error) {
	if int64(len(value)) > w.remaining {
		return 0, agentexecution.ErrOutputLimit
	}
	n, err := w.writer.Write(value)
	w.remaining -= int64(n)
	w.written += int64(n)
	return n, err
}

// Serialize a confined installation tree deterministically. os.Root and the
// existing checkpoint encoder reject links escaping that installation root.
func (b *Backend) dependencyArchive(ctx context.Context, target string, destination io.Writer) (dependencyArtifact, error) {
	var artifact dependencyArtifact
	if !validDependencyTarget(target) {
		return artifact, agentexecution.ErrInvalidPath
	}
	info, err := b.workspace.Lstat(target)
	if err != nil {
		return artifact, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return artifact, errors.New("dependency target must be a directory")
	}
	root, err := b.workspace.OpenRoot(target)
	if err != nil {
		return artifact, err
	}
	defer root.Close()
	return encodeDependencyArchive(ctx, root, target, destination)
}

func encodeDependencyArchive(ctx context.Context, root *os.Root, target string, destination io.Writer) (dependencyArtifact, error) {
	hash := sha256.New()
	limited := artifactWriter{writer: io.MultiWriter(destination, hash), remaining: maxDependencyArtifactBytes}
	compressed := gzip.NewWriter(&limited)
	archive := tar.NewWriter(compressed)
	encoder := Backend{workspace: root}
	err := encoder.writeWorkspaceArchiveMode(ctx, archive, true)
	err = errors.Join(err, archive.Close(), compressed.Close())
	if err != nil {
		return dependencyArtifact{}, err
	}
	return dependencyArtifact{Target: target, SHA256: hex.EncodeToString(hash.Sum(nil)), Size: limited.written}, nil
}

func (b *Backend) captureDependencyArtifacts(ctx context.Context, packages []languagePackage) ([]dependencyArtifact, []string, error) {
	targets := dependencyTargets(packages)
	if len(targets) > maxDependencyArtifacts {
		return nil, nil, agentexecution.ErrOutputLimit
	}
	if len(targets) == 0 {
		return nil, nil, nil
	}
	if err := b.workspace.MkdirAll(dependencyArtifactDirectory, 0o700); err != nil {
		return nil, nil, err
	}
	var artifacts []dependencyArtifact
	var uncached []string
	var total int64
	for _, target := range targets {
		artifact, err := b.captureDependencyArtifact(ctx, target)
		// Workspace-linked dependencies remain valid in a complete checkpoint,
		// without copying their linked business directory into a package artifact.
		if errors.Is(err, agentexecution.ErrInvalidPath) {
			uncached = append(uncached, target)
			continue
		}
		if err != nil {
			return artifacts, uncached, err
		}
		artifacts = append(artifacts, artifact)
		total += artifact.Size
		if total > maxDependencyArtifactTotal {
			return artifacts, uncached, agentexecution.ErrOutputLimit
		}
	}
	return artifacts, uncached, nil
}

func (b *Backend) captureDependencyArtifact(ctx context.Context, target string) (dependencyArtifact, error) {
	var artifact dependencyArtifact
	temporary, err := temporaryName(dependencyArtifactDirectory + "/capture")
	if err != nil {
		return artifact, err
	}
	file, err := b.workspace.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return artifact, err
	}
	defer b.workspace.Remove(temporary)
	artifact, err = b.dependencyArchive(ctx, target, file)
	err = errors.Join(err, file.Sync(), file.Close())
	if err != nil {
		return artifact, err
	}
	if err := b.workspace.Rename(temporary, artifact.name()); err != nil {
		return artifact, fmt.Errorf("commit dependency artifact: %w", err)
	}
	return artifact, nil
}
