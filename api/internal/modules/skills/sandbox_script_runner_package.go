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
		skillScriptsHaveThirdPartyImports(root)
}

var (
	pythonImportHintPattern = regexp.MustCompile(`(?m)^\s*(?:from|import)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	nodeImportHintPattern   = regexp.MustCompile(`(?m)^\s*(?:import\s+(?:[^'"]+\s+from\s+)?|const\s+\w+\s*=\s*require\()\s*['"]([^'"]+)['"]`)
)

func skillScriptsHaveThirdPartyImports(root string) bool {
	scriptsRoot := filepath.Join(root, "scripts")
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
			if pythonContentHasThirdPartyImport(string(content)) {
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

func pythonContentHasThirdPartyImport(content string) bool {
	for _, match := range pythonImportHintPattern.FindAllStringSubmatch(content, -1) {
		if len(match) >= 2 && !isKnownPythonStdlibRoot(match[1]) {
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
		if spec != "" && !strings.HasPrefix(spec, ".") && !strings.HasPrefix(spec, "/") && !strings.HasPrefix(spec, "node:") {
			return true
		}
	}
	return false
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
	Entrypoint           string                          `json:"entrypoint"`
	Language             string                          `json:"language"`
	DependencyProfile    string                          `json:"dependency_profile"`
	Dependencies         skillScriptManifestDependencies `json:"dependencies,omitempty"`
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

func prepareSkillScriptManifest(root string, fallbackTimeoutSeconds int) (preparedSkillScriptManifest, error) {
	manifest, err := loadSkillScriptManifest(root)
	if err != nil {
		return preparedSkillScriptManifest{}, err
	}
	if err := normalizeSkillScriptManifest(root, fallbackTimeoutSeconds, &manifest); err != nil {
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

func normalizeSkillScriptManifest(root string, fallbackTimeoutSeconds int, manifest *skillScriptManifest) error {
	if manifest == nil {
		return fmt.Errorf("skill manifest is empty")
	}
	manifest.Entrypoint = filepath.ToSlash(strings.TrimSpace(manifest.Entrypoint))
	if manifest.Entrypoint == "" {
		manifest.Entrypoint = "scripts/run.py"
	}
	if unsafeSkillManifestPath(manifest.Entrypoint) {
		return fmt.Errorf("skill manifest entrypoint escapes package root: %s", manifest.Entrypoint)
	}
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

	// Dependency profiles are selected by the platform from prepared dependency
	// requests and verified artifacts. Skill packages may declare dependencies,
	// but they must not choose the runtime profile directly.
	manifest.DependencyProfile = defaultSkillDependencyProfile
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
		path := filepath.ToSlash(strings.TrimSpace(raw))
		if path == "" {
			return fmt.Errorf("skill manifest allowed_artifact_paths contains an empty path")
		}
		if unsafeSkillManifestPath(path) || (path != "artifacts" && !strings.HasPrefix(path, "artifacts/")) {
			return fmt.Errorf("skill manifest allowed artifact path must be under artifacts: %s", path)
		}
		manifest.AllowedArtifactPaths[i] = path
	}
	if manifest.MaxArtifactCount <= 0 {
		manifest.MaxArtifactCount = 10
	}
	if manifest.MaxArtifactCount > 10 {
		return fmt.Errorf("skill manifest max_artifact_count must be between 1 and 10")
	}
	if manifest.MaxArtifactBytes <= 0 {
		manifest.MaxArtifactBytes = maxSkillScriptArtifactBytes
	}
	if manifest.MaxArtifactBytes > maxSkillScriptArtifactBytes {
		return fmt.Errorf("skill manifest max_artifact_bytes must be between 1 and %d", maxSkillScriptArtifactBytes)
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
	for index := range manifest.InputFiles {
		if err := normalizeSkillScriptInputFileSpec(&manifest.InputFiles[index]); err != nil {
			return err
		}
	}
	return nil
}

func normalizeSkillScriptInputFileSpec(spec *skillScriptInputFileSpec) error {
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
		spec.MaxBytes = maxSkillScriptInputFileBytes
	}
	if spec.MaxBytes > maxSkillScriptInputFileBytes {
		return fmt.Errorf("skill manifest input file %s max_bytes must be between 1 and %d", spec.Name, maxSkillScriptInputFileBytes)
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
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(value)))
	return clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/")
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
