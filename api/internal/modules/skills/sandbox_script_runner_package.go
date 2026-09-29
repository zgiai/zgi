package skills

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func skillPackageHasDependencyHints(root string) bool {
	for _, rel := range []string{"requirements.txt", "package.json"} {
		info, err := os.Lstat(filepath.Join(root, rel))
		if err == nil && !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			return true
		}
	}
	manifest, err := loadSkillScriptManifest(root)
	if err != nil {
		return false
	}
	return len(manifest.Dependencies.Python) > 0 ||
		len(manifest.Dependencies.Node) > 0 ||
		len(manifest.Dependencies.NodeJS) > 0 ||
		len(manifest.Dependencies.System) > 0 ||
		skillScriptsHaveThirdPartyImports(root, manifest)
}

// CustomSkillHasDependencyHints reports a conservative import-preview hint;
// it does not authorize or install dependencies.
func CustomSkillHasDependencyHints(root string) bool {
	return skillPackageHasDependencyHints(root)
}

var (
	pythonImportHintPattern = regexp.MustCompile(`(?m)^\s*(?:from|import)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	nodeImportHintPattern   = regexp.MustCompile(`(?m)(?:import\s+(?:[^'"]+\s+from\s+)?|import\s*\(|require\s*\()\s*['"]([^'"]+)['"]`)
)

func skillScriptsHaveThirdPartyImports(root string, manifest skillScriptManifest) bool {
	scriptsRoot := filepath.Join(root, "scripts")
	pythonImportRoot := skillPythonEntrypointImportRoot(root, manifest.Entrypoint)
	err := filepath.WalkDir(scriptsRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry == nil || entry.IsDir() {
			return nil
		}
		if path == scriptsRoot {
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.Mode()&os.ModeSymlink != 0 || info.Size() > 256*1024 {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		switch filepath.Ext(path) {
		case ".py":
			if pythonContentHasThirdPartyImport(string(content), pythonImportRoot) {
				return errDependencyHintFound
			}
		case ".js", ".mjs", ".cjs":
			if nodeContentHasThirdPartyImport(string(content)) {
				return errDependencyHintFound
			}
		}
		return nil
	})
	return errors.Is(err, errDependencyHintFound)
}

var errDependencyHintFound = errors.New("dependency hint found")

func pythonContentHasThirdPartyImport(content string, importRoot string) bool {
	for _, match := range pythonImportHintPattern.FindAllStringSubmatch(content, -1) {
		if len(match) < 2 {
			continue
		}
		module := strings.TrimSpace(match[1])
		if module != "" && !isKnownPythonStdlibRoot(module) && !pythonFilesystemImportIsLocal(importRoot, module) {
			return true
		}
	}
	return false
}

func skillPythonEntrypointImportRoot(root string, entrypoint string) string {
	entrypoint = normalizeSkillManifestPath(entrypoint)
	if entrypoint == "" || unsafeSkillManifestPath(entrypoint) || !strings.HasPrefix(entrypoint, "scripts/") {
		entrypoint = "scripts/run.py"
	}
	return filepath.Join(root, filepath.FromSlash(filepath.Dir(entrypoint)))
}

func pythonFilesystemImportIsLocal(importRoot string, module string) bool {
	module = strings.Split(strings.TrimSpace(module), ".")[0]
	if importRoot == "" || module == "" {
		return false
	}
	for _, candidate := range []string{
		filepath.Join(importRoot, module+".py"),
		filepath.Join(importRoot, module),
	} {
		info, err := os.Lstat(candidate)
		if err == nil && info.Mode()&os.ModeSymlink == 0 {
			return true
		}
	}
	return false
}

func nodeContentHasThirdPartyImport(content string) bool {
	for _, match := range nodeImportHintPattern.FindAllStringSubmatch(content, -1) {
		if len(match) < 2 {
			continue
		}
		spec := strings.TrimSpace(match[1])
		if spec != "" && !strings.HasPrefix(spec, ".") && !strings.HasPrefix(spec, "/") && !strings.HasPrefix(spec, "node:") && !isKnownNodeBuiltinRoot(spec) {
			return true
		}
	}
	return false
}

func isKnownNodeBuiltinRoot(name string) bool {
	root := strings.Split(strings.TrimSpace(name), "/")[0]
	switch root {
	case "assert", "async_hooks", "buffer", "child_process", "cluster", "console", "constants", "crypto", "dgram", "diagnostics_channel", "dns", "domain", "events", "fs", "http", "http2", "https", "inspector", "module", "net", "os", "path", "perf_hooks", "process", "punycode", "querystring", "readline", "repl", "stream", "string_decoder", "sys", "timers", "tls", "trace_events", "tty", "url", "util", "v8", "vm", "wasi", "worker_threads", "zlib":
		return true
	default:
		return false
	}
}

func isKnownPythonStdlibRoot(name string) bool {
	switch strings.TrimSpace(name) {
	case "", "__future__", "argparse", "asyncio", "base64", "collections", "contextlib", "csv", "dataclasses", "datetime", "decimal", "functools", "glob", "hashlib", "io", "itertools", "json", "logging", "math", "os", "pathlib", "random", "re", "shutil", "statistics", "string", "subprocess", "sys", "tempfile", "time", "typing", "uuid", "zipfile":
		return true
	default:
		return false
	}
}

func hasSkillManifest(root string) (bool, error) {
	info, err := os.Lstat(filepath.Join(root, "skill.manifest.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if info.IsDir() {
		return false, fmt.Errorf("skill.manifest.json is a directory")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("skill.manifest.json is a symlink")
	}
	return true, nil
}

func zipSkillDirectoryBase64(root string, manifestRaw []byte) (string, error) {
	if err := validateSkillPackageUploadSize(root, manifestRaw); err != nil {
		return "", err
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("skill package contains symlink: %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if filepath.ToSlash(rel) == "skill.manifest.json" && len(manifestRaw) > 0 {
			return nil
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		header.Method = zip.Deflate
		fileWriter, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		return copyFileIntoZip(path, fileWriter)
	})
	if err == nil && len(manifestRaw) > 0 {
		err = addSkillManifestToZip(writer, manifestRaw)
	}
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes()), nil
}

type skillScriptManifest struct {
	ModelCalls           int                             `json:"model_calls,omitempty"`
	Entrypoint           string                          `json:"entrypoint"`
	Language             string                          `json:"language"`
	DependencyProfile    string                          `json:"dependency_profile"`
	Dependencies         skillScriptManifestDependencies `json:"dependencies,omitempty"`
	BundledPackages      []skillScriptBundledPackage     `json:"bundled_packages,omitempty"`
	TimeoutMS            int                             `json:"timeout_ms"`
	AllowedArtifactPaths []string                        `json:"allowed_artifact_paths"`
	MaxArtifactCount     int                             `json:"max_artifact_count"`
	MaxArtifactBytes     int64                           `json:"max_artifact_bytes"`
	ResultMode           string                          `json:"result_mode"`
	InputFiles           []skillScriptInputFileSpec      `json:"input_files,omitempty"`
}

type skillScriptManifestDependencies struct {
	Python []string `json:"python,omitempty"`
	Node   []string `json:"node,omitempty"`
	NodeJS []string `json:"nodejs,omitempty"`
	System []string `json:"system,omitempty"`
}

type skillScriptBundledPackage struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type skillScriptInputFileSpec struct {
	Name       string   `json:"name"`
	Argument   string   `json:"argument"`
	Required   bool     `json:"required"`
	Multiple   bool     `json:"multiple,omitempty"`
	MaxCount   int      `json:"max_count,omitempty"`
	Extensions []string `json:"extensions,omitempty"`
	MimeTypes  []string `json:"mime_types,omitempty"`
	MaxBytes   int64    `json:"max_bytes,omitempty"`
}

type preparedSkillScriptManifest struct {
	Manifest skillScriptManifest
	Raw      []byte
}

func defaultSkillScriptManifest(fallbackTimeoutSeconds int) skillScriptManifest {
	timeoutSeconds := fallbackTimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultSkillScriptTimeoutSeconds
	}
	return skillScriptManifest{
		Entrypoint:           "scripts/run.py",
		Language:             "python3",
		DependencyProfile:    defaultSkillDependencyProfile,
		TimeoutMS:            timeoutSeconds * 1000,
		AllowedArtifactPaths: []string{"artifacts"},
		MaxArtifactCount:     maxSkillScriptArtifactCount,
		MaxArtifactBytes:     maxSkillScriptArtifactBytes,
		ResultMode:           "mixed",
	}
}

func prepareSkillScriptManifest(root string, fallbackTimeoutSeconds int, allowManagedProfile bool) (preparedSkillScriptManifest, error) {
	manifest, err := loadSkillScriptManifest(root)
	if err != nil {
		return preparedSkillScriptManifest{}, err
	}
	if err := normalizeSkillScriptManifest(root, fallbackTimeoutSeconds, allowManagedProfile, &manifest); err != nil {
		return preparedSkillScriptManifest{}, err
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		return preparedSkillScriptManifest{}, err
	}
	return preparedSkillScriptManifest{Manifest: manifest, Raw: raw}, nil
}

func loadSkillScriptManifest(root string) (skillScriptManifest, error) {
	manifestPath := filepath.Join(root, "skill.manifest.json")
	info, err := os.Lstat(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return skillScriptManifest{}, nil
		}
		return skillScriptManifest{}, fmt.Errorf("failed to inspect skill manifest: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return skillScriptManifest{}, fmt.Errorf("skill.manifest.json must not be a symlink")
	}
	if info.IsDir() {
		return skillScriptManifest{}, fmt.Errorf("skill.manifest.json must be a file")
	}
	if info.Size() > 64*1024 {
		return skillScriptManifest{}, fmt.Errorf("skill.manifest.json exceeds max size of 65536 bytes")
	}
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return skillScriptManifest{}, fmt.Errorf("failed to read skill manifest: %w", err)
	}
	var manifest skillScriptManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return skillScriptManifest{}, fmt.Errorf("invalid skill.manifest.json: %w", err)
	}
	return manifest, nil
}

func normalizeSkillScriptManifest(root string, fallbackTimeoutSeconds int, allowManagedProfile bool, manifest *skillScriptManifest) error {
	if manifest == nil {
		return fmt.Errorf("skill manifest is empty")
	}
	if manifest.ModelCalls < 0 || manifest.ModelCalls > 3 {
		return fmt.Errorf("model_calls must be between 0 and 3")
	}
	manifest.Entrypoint = strings.TrimSpace(manifest.Entrypoint)
	if manifest.Entrypoint == "" {
		manifest.Entrypoint = "scripts/run.py"
	}
	if unsafeSkillManifestPath(manifest.Entrypoint) {
		return fmt.Errorf("skill manifest entrypoint escapes package root: %s", manifest.Entrypoint)
	}
	manifest.Entrypoint = normalizeSkillManifestPath(manifest.Entrypoint)
	if manifest.Entrypoint == "scripts" || !strings.HasPrefix(manifest.Entrypoint, "scripts/") {
		return fmt.Errorf("skill manifest entrypoint must be under scripts/: %s", manifest.Entrypoint)
	}
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(manifest.Entrypoint)))
	if err != nil || info.IsDir() {
		return fmt.Errorf("skill manifest entrypoint is missing from package: %s", manifest.Entrypoint)
	}

	manifest.Language = normalizeSkillScriptLanguage(manifest.Language)
	if manifest.Language == "" {
		manifest.Language = "python3"
	}
	if manifest.Language != "python3" && manifest.Language != "nodejs" {
		return fmt.Errorf("skill manifest language must be python3 or nodejs for API run_script: %s", manifest.Language)
	}
	if manifest.Language == "python3" && filepath.Ext(manifest.Entrypoint) != ".py" {
		return fmt.Errorf("skill manifest python3 entrypoint must use .py: %s", manifest.Entrypoint)
	}
	if manifest.Language == "nodejs" && filepath.Ext(manifest.Entrypoint) != ".js" {
		return fmt.Errorf("skill manifest nodejs entrypoint must use .js: %s", manifest.Entrypoint)
	}
	if err := normalizeSkillScriptBundledPackages(root, manifest); err != nil {
		return err
	}

	// Only a server grant matched to the organization and package content may
	// authorize a declared managed profile. Package source and name confer no
	// execution privilege.
	declaredProfile := strings.ToLower(strings.TrimSpace(manifest.DependencyProfile))
	if !allowManagedProfile && declaredProfile != "" && declaredProfile != defaultSkillDependencyProfile {
		return fmt.Errorf("skill dependency profile is not authorized for this organization and package")
	}
	if !allowManagedProfile || declaredProfile == "" {
		manifest.DependencyProfile = defaultSkillDependencyProfile
	} else {
		if !regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`).MatchString(declaredProfile) {
			return fmt.Errorf("skill manifest dependency_profile is invalid: %s", declaredProfile)
		}
		manifest.DependencyProfile = declaredProfile
	}
	if manifest.TimeoutMS <= 0 {
		timeoutSeconds := fallbackTimeoutSeconds
		if timeoutSeconds <= 0 {
			timeoutSeconds = defaultSkillScriptTimeoutSeconds
		}
		manifest.TimeoutMS = timeoutSeconds * 1000
	}
	if manifest.TimeoutMS <= 0 || manifest.TimeoutMS > 300000 {
		return fmt.Errorf("skill manifest timeout_ms must be between 1 and 300000")
	}
	if len(manifest.AllowedArtifactPaths) == 0 {
		manifest.AllowedArtifactPaths = []string{"artifacts"}
	}
	for i, raw := range manifest.AllowedArtifactPaths {
		if strings.TrimSpace(raw) == "" {
			return fmt.Errorf("skill manifest allowed_artifact_paths contains an empty path")
		}
		if unsafeSkillManifestPath(raw) {
			return fmt.Errorf("skill manifest allowed artifact path must be under artifacts: %s", raw)
		}
		path := normalizeSkillManifestPath(raw)
		if path != "artifacts" && !strings.HasPrefix(path, "artifacts/") {
			return fmt.Errorf("skill manifest allowed artifact path must be under artifacts: %s", path)
		}
		manifest.AllowedArtifactPaths[i] = path
	}
	artifactCountLimit := maxSkillScriptArtifactCount
	artifactBytesLimit := int64(maxSkillScriptArtifactBytes)
	inputFileBytesLimit := int64(maxSkillScriptInputFileBytes)
	if allowManagedProfile {
		artifactCountLimit = maxManagedSkillScriptArtifactCount
		artifactBytesLimit = maxManagedSkillScriptArtifactBytes
		inputFileBytesLimit = maxManagedSkillScriptInputFileBytes
	}
	if manifest.MaxArtifactCount <= 0 {
		manifest.MaxArtifactCount = maxSkillScriptArtifactCount
	}
	if manifest.MaxArtifactCount > artifactCountLimit {
		return fmt.Errorf("skill manifest max_artifact_count must be between 1 and %d", artifactCountLimit)
	}
	if manifest.MaxArtifactBytes <= 0 {
		manifest.MaxArtifactBytes = maxSkillScriptArtifactBytes
	}
	if manifest.MaxArtifactBytes > artifactBytesLimit {
		return fmt.Errorf("skill manifest max_artifact_bytes must be between 1 and %d", artifactBytesLimit)
	}
	manifest.ResultMode = strings.TrimSpace(manifest.ResultMode)
	if manifest.ResultMode == "" {
		manifest.ResultMode = "mixed"
	}
	switch manifest.ResultMode {
	case "stdout_json", "stdout_text", "artifacts", "mixed":
	default:
		return fmt.Errorf("skill manifest result_mode must be stdout_json, stdout_text, artifacts, or mixed")
	}
	if len(manifest.InputFiles) > maxSkillScriptInputFileCount {
		return fmt.Errorf("skill manifest input_files must contain at most %d files", maxSkillScriptInputFileCount)
	}
	seenInputNames := make(map[string]struct{}, len(manifest.InputFiles))
	seenInputArguments := make(map[string]struct{}, len(manifest.InputFiles))
	for index := range manifest.InputFiles {
		if err := normalizeSkillScriptInputFileSpec(&manifest.InputFiles[index], inputFileBytesLimit); err != nil {
			return err
		}
		spec := manifest.InputFiles[index]
		if _, exists := seenInputNames[spec.Name]; exists {
			return fmt.Errorf("skill manifest input file name is duplicated: %s", spec.Name)
		}
		if _, exists := seenInputArguments[spec.Argument]; exists {
			return fmt.Errorf("skill manifest input file argument is duplicated: %s", spec.Argument)
		}
		seenInputNames[spec.Name] = struct{}{}
		seenInputArguments[spec.Argument] = struct{}{}
	}
	return nil
}

var (
	pythonBundledPackageNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	nodeBundledPackageNamePattern   = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*$`)
)

func normalizeSkillScriptBundledPackages(root string, manifest *skillScriptManifest) error {
	seenNames := make(map[string]struct{}, len(manifest.BundledPackages))
	seenPaths := make(map[string]struct{}, len(manifest.BundledPackages))
	for index := range manifest.BundledPackages {
		item := &manifest.BundledPackages[index]
		item.Name = strings.TrimSpace(item.Name)
		item.Path = strings.TrimSpace(item.Path)
		if item.Name == "" {
			return errors.New("skill manifest bundled package name is required")
		}
		if !validBundledPackageName(manifest.Language, item.Name) {
			return fmt.Errorf("skill manifest bundled package name is invalid for %s: %s", manifest.Language, item.Name)
		}
		if unsafeSkillManifestPath(item.Path) {
			return fmt.Errorf("skill manifest bundled package path must be under scripts/: %s", item.Path)
		}
		item.Path = normalizeSkillManifestPath(item.Path)
		if item.Path == "scripts" || !strings.HasPrefix(item.Path, "scripts/") {
			return fmt.Errorf("skill manifest bundled package path must be under scripts/: %s", item.Path)
		}
		nameKey := strings.ToLower(item.Name)
		pathKey := strings.ToLower(item.Path)
		if _, exists := seenNames[nameKey]; exists {
			return fmt.Errorf("skill manifest bundled package name is duplicated: %s", item.Name)
		}
		if _, exists := seenPaths[pathKey]; exists {
			return fmt.Errorf("skill manifest bundled package path is duplicated: %s", item.Path)
		}
		seenNames[nameKey] = struct{}{}
		seenPaths[pathKey] = struct{}{}

		target := filepath.Join(root, filepath.FromSlash(item.Path))
		targetInfo, err := validateBundledPackageTarget(root, target)
		if err != nil {
			return fmt.Errorf("skill manifest bundled package %s: %w", item.Name, err)
		}
		if manifest.Language == "python3" {
			importRoot := skillPythonEntrypointImportRoot(root, manifest.Entrypoint)
			expected := filepath.Join(importRoot, item.Name)
			target = filepath.Clean(target)
			expected = filepath.Clean(expected)
			switch target {
			case expected:
				if !targetInfo.IsDir() {
					return fmt.Errorf("skill manifest bundled Python package %s must use .py when it is a regular file", item.Name)
				}
			case expected + ".py":
				if !targetInfo.Mode().IsRegular() {
					return fmt.Errorf("skill manifest bundled Python package %s path ending in .py must be a regular file", item.Name)
				}
			default:
				return fmt.Errorf("skill manifest bundled Python package %s must resolve from the entrypoint directory", item.Name)
			}
		}
	}
	return nil
}

func validBundledPackageName(language string, name string) bool {
	if language == "python3" {
		return pythonBundledPackageNamePattern.MatchString(name)
	}
	return nodeBundledPackageNamePattern.MatchString(name)
}

func validateBundledPackageTarget(root string, target string) (os.FileInfo, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, errors.New("path cannot be resolved")
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return nil, errors.New("path cannot be resolved")
	}
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return nil, errors.New("path cannot be resolved")
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, errors.New("path escapes package root")
	}
	current := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, errors.New("path is missing from package")
			}
			return nil, errors.New("path cannot be inspected")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("path contains a symlink")
		}
	}
	info, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("path is missing from package")
		}
		return nil, errors.New("path cannot be inspected")
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return nil, errors.New("path must be a regular file or directory")
	}
	return info, nil
}

func normalizeSkillScriptInputFileSpec(spec *skillScriptInputFileSpec, maxBytesLimit int64) error {
	if spec == nil {
		return fmt.Errorf("skill manifest input_files contains an empty item")
	}
	spec.Name = strings.TrimSpace(spec.Name)
	if !safeSkillInputSegment(spec.Name) {
		return fmt.Errorf("skill manifest input file name must be a safe path segment: %s", spec.Name)
	}
	spec.Argument = strings.TrimSpace(spec.Argument)
	if spec.Argument == "" {
		return fmt.Errorf("skill manifest input file %s argument is required", spec.Name)
	}
	if strings.ContainsAny(spec.Argument, "/\\") || spec.Argument == "." || spec.Argument == ".." {
		return fmt.Errorf("skill manifest input file %s argument is invalid: %s", spec.Name, spec.Argument)
	}
	for i, extension := range spec.Extensions {
		normalized := strings.ToLower(strings.TrimSpace(extension))
		if normalized == "" {
			return fmt.Errorf("skill manifest input file %s extension is empty", spec.Name)
		}
		if !strings.HasPrefix(normalized, ".") {
			normalized = "." + normalized
		}
		if strings.ContainsAny(normalized, `/\`) || normalized == "." || strings.Contains(normalized, "..") {
			return fmt.Errorf("skill manifest input file %s extension is invalid: %s", spec.Name, extension)
		}
		spec.Extensions[i] = normalized
	}
	for i, mimeType := range spec.MimeTypes {
		normalized := strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
		if normalized == "" || strings.ContainsAny(normalized, " \t\r\n") {
			return fmt.Errorf("skill manifest input file %s mime type is invalid: %s", spec.Name, mimeType)
		}
		spec.MimeTypes[i] = normalized
	}
	if spec.MaxBytes <= 0 {
		spec.MaxBytes = maxBytesLimit
	}
	if spec.MaxBytes > maxBytesLimit {
		return fmt.Errorf("skill manifest input file %s max_bytes must be between 1 and %d", spec.Name, maxBytesLimit)
	}
	if spec.Multiple {
		if spec.MaxCount <= 0 {
			spec.MaxCount = maxSkillScriptInputFileCount
		}
		if spec.MaxCount > maxSkillScriptInputFileCount {
			return fmt.Errorf("skill manifest input file %s max_count must be between 1 and %d", spec.Name, maxSkillScriptInputFileCount)
		}
	} else if spec.MaxCount < 0 {
		return fmt.Errorf("skill manifest input file %s max_count must not be negative", spec.Name)
	}
	return nil
}

func safeSkillInputSegment(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	if strings.ContainsAny(value, `/\`) {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func copyFileIntoZip(path string, writer io.Writer) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(writer, file)
	return err
}

func addSkillManifestToZip(writer *zip.Writer, manifestRaw []byte) error {
	header := &zip.FileHeader{
		Name:   "skill.manifest.json",
		Method: zip.Deflate,
	}
	header.SetMode(0o644)
	fileWriter, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = fileWriter.Write(manifestRaw)
	return err
}

func normalizeSkillScriptLanguage(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return ""
	case "python", "python3":
		return "python3"
	case "node", "nodejs", "javascript":
		return "nodejs"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func unsafeSkillManifestPath(value string) bool {
	raw := strings.ReplaceAll(strings.TrimSpace(value), `\`, "/")
	if raw == "" || strings.HasPrefix(raw, "/") || filepath.IsAbs(filepath.FromSlash(raw)) {
		return true
	}
	for _, part := range strings.Split(raw, "/") {
		if part == ".." {
			return true
		}
	}
	clean := normalizeSkillManifestPath(raw)
	return clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/")
}

func normalizeSkillManifestPath(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), `\`, "/")
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
}

func skillScriptTimeoutSeconds(timeoutMS int) int {
	if timeoutMS <= 0 {
		return defaultSkillScriptTimeoutSeconds
	}
	return (timeoutMS + 999) / 1000
}

func skillManifestAllowsArtifactPath(manifest skillScriptManifest, value string) bool {
	path := filepath.ToSlash(strings.TrimSpace(value))
	if path == "" {
		return false
	}
	for _, allowed := range manifest.AllowedArtifactPaths {
		allowed = strings.TrimSuffix(filepath.ToSlash(strings.TrimSpace(allowed)), "/")
		if allowed == "" {
			continue
		}
		if path == allowed || strings.HasPrefix(path, allowed+"/") {
			return true
		}
	}
	return false
}
