package profilebuilder

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/profilecatalog"
)

type Options struct {
	ProfileName string
	SourceDir   string
	OutputDir   string
	DryRun      bool
	Force       bool
}

type Result struct {
	ProfileName        string   `json:"profile_name"`
	ProfileVersion     string   `json:"profile_version"`
	SourceDir          string   `json:"source_dir"`
	OutputDir          string   `json:"output_dir,omitempty"`
	DryRun             bool     `json:"dry_run"`
	Steps              []string `json:"steps"`
	Checksum           string   `json:"checksum,omitempty"`
	SizeBytes          int64    `json:"size_bytes,omitempty"`
	VerificationPassed bool     `json:"verification_passed"`
}

type builtManifest struct {
	profilecatalog.Profile
	Build buildMetadata `json:"build"`
}

type buildMetadata struct {
	Checksum           string    `json:"checksum"`
	SizeBytes          int64     `json:"size_bytes"`
	BuiltAt            time.Time `json:"built_at"`
	VerificationPassed bool      `json:"verification_passed"`
	Builder            string    `json:"builder"`
}

func Build(opts Options) (Result, error) {
	opts.ProfileName = strings.TrimSpace(opts.ProfileName)
	if opts.ProfileName == "" {
		return Result{}, errors.New("profile name is required")
	}
	if opts.SourceDir == "" {
		opts.SourceDir = "profiles"
	}
	if opts.OutputDir == "" {
		opts.OutputDir = filepath.Join(".profile-build", "profiles")
	}
	sourceDir, err := filepath.Abs(opts.SourceDir)
	if err != nil {
		return Result{}, fmt.Errorf("resolve source directory: %w", err)
	}
	outputDir, err := filepath.Abs(opts.OutputDir)
	if err != nil {
		return Result{}, fmt.Errorf("resolve output directory: %w", err)
	}
	opts.SourceDir = sourceDir
	opts.OutputDir = outputDir

	profiles, err := profilecatalog.Load(os.DirFS(opts.SourceDir))
	if err != nil {
		return Result{}, err
	}
	profile, ok := findProfile(profiles, opts.ProfileName)
	if !ok {
		return Result{}, fmt.Errorf("profile source not found: %s", opts.ProfileName)
	}

	sourceProfileDir := filepath.Join(opts.SourceDir, profile.Name)
	result := Result{
		ProfileName:    profile.Name,
		ProfileVersion: profile.Version,
		SourceDir:      sourceProfileDir,
		DryRun:         opts.DryRun,
		Steps:          planSteps(sourceProfileDir),
	}
	if err := rejectSymlinks(sourceProfileDir); err != nil {
		return result, err
	}
	if opts.DryRun {
		return result, nil
	}

	finalDir := filepath.Join(opts.OutputDir, profile.Name)
	tmpDir := finalDir + ".tmp"
	if err := prepareOutput(finalDir, tmpDir, opts.Force); err != nil {
		return result, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tmpDir)
		}
	}()

	if err := copySource(sourceProfileDir, tmpDir); err != nil {
		return result, err
	}
	if err := buildPython(tmpDir); err != nil {
		return result, err
	}
	if err := buildNode(tmpDir); err != nil {
		return result, err
	}
	if err := verifyPython(tmpDir); err != nil {
		return result, err
	}
	if err := verifyNode(tmpDir); err != nil {
		return result, err
	}

	checksum, size, err := checksumDir(tmpDir)
	if err != nil {
		return result, err
	}
	result.OutputDir = finalDir
	result.Checksum = checksum
	result.SizeBytes = size
	result.VerificationPassed = true

	manifest := builtManifest{
		Profile: profile,
		Build: buildMetadata{
			Checksum:           checksum,
			SizeBytes:          size,
			BuiltAt:            time.Now().UTC(),
			VerificationPassed: true,
			Builder:            "zgi-sandbox-profilebuilder",
		},
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return result, err
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "manifest.json"), append(raw, '\n'), 0o644); err != nil {
		return result, err
	}

	if err := os.MkdirAll(filepath.Dir(finalDir), 0o755); err != nil {
		return result, err
	}
	if opts.Force {
		if err := os.RemoveAll(finalDir); err != nil {
			return result, err
		}
	}
	if err := os.Rename(tmpDir, finalDir); err != nil {
		return result, err
	}
	cleanup = false
	return result, nil
}

func findProfile(profiles []profilecatalog.Profile, name string) (profilecatalog.Profile, bool) {
	for _, profile := range profiles {
		if profile.Name == name {
			return profile, true
		}
	}
	return profilecatalog.Profile{}, false
}

func planSteps(sourceProfileDir string) []string {
	steps := []string{"validate manifest", "reject source symlinks", "copy source files"}
	if fileExists(filepath.Join(sourceProfileDir, "requirements.lock")) {
		steps = append(steps, "sync python virtual environment")
	}
	if fileExists(filepath.Join(sourceProfileDir, "pnpm-lock.yaml")) {
		steps = append(steps, "install node modules from lockfile")
	}
	if fileExists(filepath.Join(sourceProfileDir, "verify.py")) {
		steps = append(steps, "run python verification")
	}
	if fileExists(filepath.Join(sourceProfileDir, "verify-node.mjs")) {
		steps = append(steps, "run node verification")
	}
	steps = append(steps, "emit built manifest")
	return steps
}

func rejectSymlinks(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			rel, _ := filepath.Rel(root, path)
			return fmt.Errorf("profile source contains symlink: %s", filepath.ToSlash(rel))
		}
		return nil
	})
}

func prepareOutput(finalDir, tmpDir string, force bool) error {
	if !force {
		if _, err := os.Stat(finalDir); err == nil {
			return fmt.Errorf("profile output already exists: %s", finalDir)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.RemoveAll(tmpDir); err != nil {
		return err
	}
	return os.MkdirAll(tmpDir, 0o755)
}

func copySource(sourceDir, outputDir string) error {
	return filepath.WalkDir(sourceDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(outputDir, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(source, target string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func buildPython(profileDir string) error {
	lockfile := filepath.Join(profileDir, "requirements.lock")
	if !fileExists(lockfile) {
		return nil
	}
	if err := run(profileDir, "python3", "-m", "venv", filepath.Join(profileDir, "venv")); err != nil {
		return err
	}
	python := filepath.Join(profileDir, "venv", "bin", "python")
	if uv, err := exec.LookPath("uv"); err == nil {
		return run(profileDir, uv, "pip", "sync", "--python", python, lockfile)
	}
	return run(profileDir, python, "-m", "pip", "install", "--disable-pip-version-check", "-r", lockfile)
}

func buildNode(profileDir string) error {
	if !fileExists(filepath.Join(profileDir, "pnpm-lock.yaml")) {
		return nil
	}
	pnpm, err := exec.LookPath("pnpm")
	if err != nil {
		return errors.New("pnpm is required to build node profile dependencies")
	}
	return run(profileDir, pnpm, nodeInstallArgs()...)
}

func nodeInstallArgs() []string {
	return []string{
		"install",
		"--prod",
		"--frozen-lockfile",
		"--config.node-linker=hoisted",
		"--config.prefer-symlinked-executables=false",
	}
}

func verifyPython(profileDir string) error {
	verify := filepath.Join(profileDir, "verify.py")
	if !fileExists(verify) {
		return nil
	}
	python := "python3"
	env := []string{"PYTHONNOUSERSITE=1"}
	if fileExists(filepath.Join(profileDir, "venv", "bin", "python")) {
		python = filepath.Join(profileDir, "venv", "bin", "python")
		env = append(env, "PATH="+filepath.Join(profileDir, "venv", "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	return runWithEnv(profileDir, env, python, verify)
}

func verifyNode(profileDir string) error {
	verify := filepath.Join(profileDir, "verify-node.mjs")
	if !fileExists(verify) {
		return nil
	}
	env := []string{}
	nodeModules := filepath.Join(profileDir, "node_modules")
	if fileExists(nodeModules) {
		env = append(env,
			"NODE_PATH="+nodeModules,
			"PATH="+filepath.Join(nodeModules, ".bin")+string(os.PathListSeparator)+os.Getenv("PATH"),
		)
	}
	return runWithEnv(profileDir, env, "node", verify)
}

func run(dir, name string, args ...string) error {
	return runWithEnv(dir, nil, name, args...)
}

func runWithEnv(dir string, extraEnv []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = mergeEnv(os.Environ(), extraEnv)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s failed: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func mergeEnv(base []string, overrides []string) []string {
	if len(overrides) == 0 {
		return base
	}
	merged := slices.Clone(base)
	indexByKey := map[string]int{}
	for index, item := range merged {
		key, _, ok := strings.Cut(item, "=")
		if ok {
			indexByKey[key] = index
		}
	}
	for _, item := range overrides {
		key, _, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		if index, exists := indexByKey[key]; exists {
			merged[index] = item
			continue
		}
		indexByKey[key] = len(merged)
		merged = append(merged, item)
	}
	return merged
}

func checksumDir(root string) (string, int64, error) {
	var files []string
	var size int64
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if filepath.ToSlash(rel) == "manifest.json" {
			return nil
		}
		files = append(files, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		size += info.Size()
		return nil
	}); err != nil {
		return "", 0, err
	}
	slices.Sort(files)
	hash := sha256.New()
	for _, rel := range files {
		hash.Write([]byte(filepath.ToSlash(rel)))
		hash.Write([]byte{0})
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return "", 0, err
		}
		hash.Write(raw)
		hash.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), size, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
