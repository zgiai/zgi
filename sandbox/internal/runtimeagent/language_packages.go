package runtimeagent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

const (
	maxLanguagePackages      = 4096
	maxPackageMetadataBytes  = 256 << 10
	maxLanguageMetadataBytes = 16 << 20
)

var (
	pythonDistributionName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,213}$`)
	// Existing npm packages can retain uppercase names even though publication
	// rules for new packages are stricter. Metadata is never an install command.
	nodeDistributionName        = regexp.MustCompile(`^(?:@[A-Za-z0-9._-]+/)?[A-Za-z0-9_][A-Za-z0-9._-]{0,213}$`)
	languageDistributionVersion = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+!_~\-]{0,255}$`)
)

// Paths distinguish nested Node installations of the same package. These are
// metadata consistency records, not privileged install arguments or attestations.
type languagePackage struct {
	Language string `json:"language"`
	Path     string `json:"path"`
	Name     string `json:"name"`
	Version  string `json:"version"`
}

func pythonMetadataPath(name string) bool {
	parts := strings.Split(name, "/")
	return len(parts) == 7 && parts[0] == sessionHomeDirectory && parts[1] == ".venv" &&
		parts[2] == "lib" && strings.HasPrefix(parts[3], "python3.") && parts[4] == "site-packages" &&
		(strings.HasSuffix(parts[5], ".dist-info") && parts[6] == "METADATA" || strings.HasSuffix(parts[5], ".egg-info") && parts[6] == "PKG-INFO")
}

func nodePackageDirectory(name string) bool {
	parts := strings.Split(name, "/")
	n := len(parts)
	if n >= 2 && parts[n-2] == "node_modules" {
		return !strings.HasPrefix(parts[n-1], ".") && !strings.HasPrefix(parts[n-1], "@")
	}
	return n >= 3 && parts[n-3] == "node_modules" && strings.HasPrefix(parts[n-2], "@") && !strings.HasPrefix(parts[n-1], ".")
}

func (p languagePackage) valid() bool {
	clean, err := cleanRelativePath(p.Path, false)
	if err != nil || clean != p.Path || len(p.Path) > 4096 || !languageDistributionVersion.MatchString(p.Version) {
		return false
	}
	switch p.Language {
	case "python":
		return pythonMetadataPath(p.Path) && pythonDistributionName.MatchString(p.Name)
	case "node":
		return path.Base(p.Path) == "package.json" && nodePackageDirectory(path.Dir(p.Path)) && len(p.Name) <= 214 && nodeDistributionName.MatchString(p.Name)
	default:
		return false
	}
}

// Read only bounded, regular metadata through os.Root. Never execute a session
// interpreter, npm hook, .pth file or user-controlled build as the root Agent.
func (b *Backend) installedLanguagePackages(ctx context.Context) ([]languagePackage, error) {
	var packages []languagePackage
	var total int
	entries := 0
	err := fs.WalkDir(b.workspace.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		entries++
		if entries > maxWorkspaceEntries+1 {
			return agentexecution.ErrOutputLimit
		}
		language := ""
		if (entry.IsDir() || entry.Type()&os.ModeSymlink != 0) && pythonMetadataPath(path.Join(name, "METADATA")) {
			name, language = path.Join(name, "METADATA"), "python"
		} else if (entry.IsDir() || entry.Type()&os.ModeSymlink != 0) && pythonMetadataPath(path.Join(name, "PKG-INFO")) {
			name, language = path.Join(name, "PKG-INFO"), "python"
		} else if (entry.IsDir() || entry.Type()&os.ModeSymlink != 0) && nodePackageDirectory(name) {
			// pnpm and npm workspace links are allowed only within os.Root. Record
			// the resolved metadata without walking an unbounded link graph.
			name, language = path.Join(name, "package.json"), "node"
		} else {
			return nil
		}
		raw, err := readRegularWorkspaceFile(b.workspace, name, maxPackageMetadataBytes)
		if err != nil {
			return err
		}
		total += len(raw)
		if total > maxLanguageMetadataBytes || len(packages) >= maxLanguagePackages {
			return agentexecution.ErrOutputLimit
		}
		pkg := languagePackage{Language: language, Path: name}
		if language == "node" {
			var metadata struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}
			if err := json.Unmarshal(raw, &metadata); err != nil {
				return errors.New("invalid Node package metadata")
			}
			pkg.Name, pkg.Version = metadata.Name, metadata.Version
		} else {
			// Core metadata fields are in the header, not the long description.
			for line := range strings.SplitSeq(string(raw), "\n") {
				line = strings.TrimSuffix(line, "\r")
				if line == "" {
					break
				}
				key, value, ok := strings.Cut(line, ":")
				if !ok {
					continue
				}
				switch strings.ToLower(key) {
				case "name":
					if pkg.Name != "" {
						return errors.New("duplicate Python package name")
					}
					pkg.Name = strings.TrimSpace(value)
				case "version":
					if pkg.Version != "" {
						return errors.New("duplicate Python package version")
					}
					pkg.Version = strings.TrimSpace(value)
				}
			}
		}
		if !pkg.valid() {
			return errors.New("invalid language package identity or version")
		}
		packages = append(packages, pkg)
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(packages, func(a, b languagePackage) int { return strings.Compare(a.Path, b.Path) })
	return packages, nil
}

func readRegularWorkspaceFile(root *os.Root, name string, maximum int64) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("package metadata must be a regular file")
	}
	file, err := openRegularWorkspaceFile(root, name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, errors.New("package metadata changed during open")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maximum {
		return nil, agentexecution.ErrOutputLimit
	}
	return raw, nil
}
