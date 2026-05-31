package executor

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const dependencyPrepareSchemaVersion = 1

type DependencyPrepareRequest struct {
	OrganizationID string `json:"organization_id,omitempty"`
	ArchiveBase64  string `json:"archive_base64"`
	Format         string `json:"format"`
	StripRoot      bool   `json:"strip_root"`
	Language       string `json:"language,omitempty"`
	BaseRuntime    string `json:"base_runtime,omitempty"`
}

type DependencyPrepareResult struct {
	Status       string               `json:"status"`
	Fingerprint  string               `json:"fingerprint"`
	Request      DependencyRequest    `json:"dependency_request"`
	Packages     []DetectedDependency `json:"packages"`
	PackageCount int                  `json:"package_count"`
	Sources      []string             `json:"sources"`
	Warnings     []string             `json:"warnings,omitempty"`
	NextAction   string               `json:"next_action"`
}

type DependencyRequest struct {
	SchemaVersion int                  `json:"schema_version"`
	Language      string               `json:"language"`
	BaseRuntime   string               `json:"base_runtime"`
	Python        []DetectedDependency `json:"python,omitempty"`
	Node          []DetectedDependency `json:"node,omitempty"`
	System        []DetectedDependency `json:"system,omitempty"`
}

type DetectedDependency struct {
	Ecosystem string   `json:"ecosystem"`
	Name      string   `json:"name"`
	Version   string   `json:"version,omitempty"`
	Source    []string `json:"source,omitempty"`
}

type manifestDependencyBlock struct {
	Python []string `json:"python"`
	Node   []string `json:"node"`
	NodeJS []string `json:"nodejs"`
	System []string `json:"system"`
}

type dependencyScanManifest struct {
	Language     string                  `json:"language"`
	Dependencies manifestDependencyBlock `json:"dependencies"`
}

func (s *Service) PrepareDependencies(req DependencyPrepareRequest) (DependencyPrepareResult, error) {
	if strings.TrimSpace(req.ArchiveBase64) == "" {
		return DependencyPrepareResult{}, fmt.Errorf("archive_base64 is required")
	}
	if !strings.EqualFold(strings.TrimSpace(req.Format), "zip") {
		return DependencyPrepareResult{}, fmt.Errorf("unsupported archive format")
	}

	archiveBytes, err := base64.StdEncoding.DecodeString(req.ArchiveBase64)
	if err != nil {
		return DependencyPrepareResult{}, fmt.Errorf("invalid archive_base64: %w", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
	if err != nil {
		return DependencyPrepareResult{}, fmt.Errorf("invalid zip archive: %w", err)
	}
	entries, err := normalizeArchiveEntries(reader.File, req.StripRoot)
	if err != nil {
		return DependencyPrepareResult{}, err
	}
	maxFileSize := int64(256 * 1024)
	if s != nil && s.policy != nil {
		maxFileSize = s.policy.MaxFileSizeBytes()
	}
	if _, err := validateArchiveEntriesWithinLimits(entries, archiveLimits{
		maxFiles:     256,
		maxFileSize:  maxFileSize,
		maxTotalSize: maxFileSize * 256,
	}); err != nil {
		return DependencyPrepareResult{}, err
	}

	baseRuntime := strings.TrimSpace(req.BaseRuntime)
	if baseRuntime == "" && s != nil && s.policy != nil {
		baseRuntime = s.policy.RuntimeBackend()
	}
	if baseRuntime == "" {
		baseRuntime = "linux-secure"
	}

	result, err := scanDependencyRequest(entries, normalizeSkillLanguage(req.Language), baseRuntime)
	if err != nil {
		return DependencyPrepareResult{}, err
	}
	if len(result.Packages) == 0 {
		result.Status = "ready"
		result.NextAction = "use_default_dependency_profile"
	} else {
		result.Status = "build_required"
		result.NextAction = "queue_dependency_build"
	}
	return result, nil
}

func scanDependencyRequest(entries []archiveEntry, requestedLanguage string, baseRuntime string) (DependencyPrepareResult, error) {
	files := make(map[string]*zip.File, len(entries))
	for _, entry := range entries {
		files[filepath.ToSlash(entry.name)] = entry.file
	}

	language := requestedLanguage
	packages := map[string]DetectedDependency{}
	sourceSet := map[string]struct{}{}
	warnings := []string{}

	if file := files["skill.manifest.json"]; file != nil {
		content, err := readZipFile(file, 64*1024)
		if err != nil {
			return DependencyPrepareResult{}, err
		}
		var manifest dependencyScanManifest
		if err := json.Unmarshal(content, &manifest); err != nil {
			return DependencyPrepareResult{}, fmt.Errorf("invalid skill.manifest.json dependencies: %w", err)
		}
		if language == "" {
			language = normalizeSkillLanguage(manifest.Language)
		}
		addPackageSpecs(packages, sourceSet, "python3", manifest.Dependencies.Python, "skill.manifest.json")
		nodeSpecs := append([]string{}, manifest.Dependencies.Node...)
		nodeSpecs = append(nodeSpecs, manifest.Dependencies.NodeJS...)
		addPackageSpecs(packages, sourceSet, "nodejs", nodeSpecs, "skill.manifest.json")
		addPackageSpecs(packages, sourceSet, "system", manifest.Dependencies.System, "skill.manifest.json")
	}

	if language == "" {
		language = inferPrimaryLanguage(files)
	}
	if language == "" {
		language = "python3"
		warnings = append(warnings, "language defaulted to python3")
	}

	if file := files["requirements.txt"]; file != nil {
		specs, err := readRequirementsSpecs(file)
		if err != nil {
			return DependencyPrepareResult{}, err
		}
		addPackageSpecs(packages, sourceSet, "python3", specs, "requirements.txt")
	}
	if file := files["package.json"]; file != nil {
		specs, err := readPackageJSONSpecs(file)
		if err != nil {
			return DependencyPrepareResult{}, err
		}
		addPackageSpecs(packages, sourceSet, "nodejs", specs, "package.json")
	}

	for path, file := range files {
		switch filepath.Ext(path) {
		case ".py":
			if strings.HasPrefix(path, "scripts/") {
				specs, err := scanPythonImports(file)
				if err != nil {
					return DependencyPrepareResult{}, err
				}
				addPackageSpecs(packages, sourceSet, "python3", specs, "static_import:"+path)
			}
		case ".js", ".mjs", ".cjs":
			if strings.HasPrefix(path, "scripts/") {
				specs, err := scanNodeImports(file)
				if err != nil {
					return DependencyPrepareResult{}, err
				}
				addPackageSpecs(packages, sourceSet, "nodejs", specs, "static_import:"+path)
			}
		}
	}

	request := DependencyRequest{
		SchemaVersion: dependencyPrepareSchemaVersion,
		Language:      language,
		BaseRuntime:   baseRuntime,
	}
	all := sortedDependencies(packages)
	for _, item := range all {
		switch item.Ecosystem {
		case "python3":
			request.Python = append(request.Python, item)
		case "nodejs":
			request.Node = append(request.Node, item)
		case "system":
			request.System = append(request.System, item)
		}
	}

	result := DependencyPrepareResult{
		Fingerprint:  dependencyFingerprint(request),
		Request:      request,
		Packages:     all,
		PackageCount: len(all),
		Sources:      sortedSourceSet(sourceSet),
		Warnings:     warnings,
	}
	return result, nil
}

func addPackageSpecs(packages map[string]DetectedDependency, sources map[string]struct{}, ecosystem string, specs []string, source string) {
	source = strings.TrimSpace(source)
	added := false
	for _, spec := range specs {
		dep := normalizeDependencySpec(ecosystem, spec)
		if dep.Name == "" {
			continue
		}
		added = true
		key := dep.Ecosystem + "\x00" + dep.Name
		if dep.Version != "" {
			key += "\x00" + dep.Version
		}
		existing, ok := packages[key]
		if ok {
			existing.Source = appendUniqueString(existing.Source, source)
			packages[key] = existing
			continue
		}
		if source != "" {
			dep.Source = []string{source}
		}
		packages[key] = dep
	}
	if added && source != "" {
		sources[source] = struct{}{}
	}
}

func normalizeDependencySpec(ecosystem string, spec string) DetectedDependency {
	ecosystem = normalizeDependencyEcosystem(ecosystem)
	spec = strings.TrimSpace(spec)
	spec = strings.Trim(spec, `"'`)
	if spec == "" || strings.HasPrefix(spec, "#") {
		return DetectedDependency{}
	}
	if index := strings.Index(spec, "#"); index >= 0 {
		spec = strings.TrimSpace(spec[:index])
	}
	if spec == "" || strings.HasPrefix(spec, "-") {
		return DetectedDependency{}
	}

	name := spec
	version := ""
	if ecosystem == "nodejs" {
		name, version = splitNodePackageSpec(spec)
	} else {
		for _, sep := range []string{"==", ">=", "<=", "~=", "!=", ">", "<"} {
			if index := strings.Index(spec, sep); index > 0 {
				name = strings.TrimSpace(spec[:index])
				version = strings.TrimSpace(spec[index:])
				break
			}
		}
	}
	name = normalizePackageName(ecosystem, name)
	if name == "" {
		return DetectedDependency{}
	}
	return DetectedDependency{Ecosystem: ecosystem, Name: name, Version: version}
}

func splitNodePackageSpec(spec string) (string, string) {
	spec = strings.TrimSpace(spec)
	if strings.HasPrefix(spec, "@") {
		scopeEnd := strings.Index(spec, "/")
		if scopeEnd < 0 {
			return spec, ""
		}
		versionIndex := strings.LastIndex(spec[scopeEnd+1:], "@")
		if versionIndex < 0 {
			return spec, ""
		}
		versionIndex += scopeEnd + 1
		return strings.TrimSpace(spec[:versionIndex]), strings.TrimSpace(strings.TrimPrefix(spec[versionIndex:], "@"))
	}
	if index := strings.LastIndex(spec, "@"); index > 0 {
		return strings.TrimSpace(spec[:index]), strings.TrimSpace(strings.TrimPrefix(spec[index:], "@"))
	}
	return spec, ""
}

func normalizeDependencyEcosystem(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "python", "python3":
		return "python3"
	case "node", "nodejs", "javascript":
		return "nodejs"
	case "system", "apt", "apt-get":
		return "system"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func normalizePackageName(ecosystem string, value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	if value == "" || strings.HasPrefix(value, ".") || strings.HasPrefix(value, "/") {
		return ""
	}
	if ecosystem == "nodejs" && strings.HasPrefix(value, "@") {
		parts := strings.Split(value, "/")
		if len(parts) >= 2 {
			return strings.ToLower(parts[0] + "/" + parts[1])
		}
	}
	value = strings.Split(value, "[")[0]
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if ecosystem == "python3" {
		if mapped := pythonImportPackageMap[value]; mapped != "" {
			return mapped
		}
		return strings.ToLower(strings.ReplaceAll(value, "_", "-"))
	}
	return strings.ToLower(value)
}

func readRequirementsSpecs(file *zip.File) ([]string, error) {
	content, err := readZipFile(file, 256*1024)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(content), "\n")
	specs := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "-") {
			specs = append(specs, line)
		}
	}
	return specs, nil
}

func readPackageJSONSpecs(file *zip.File) ([]string, error) {
	content, err := readZipFile(file, 256*1024)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(content, &parsed); err != nil {
		return nil, fmt.Errorf("invalid package.json: %w", err)
	}
	specs := make([]string, 0, len(parsed.Dependencies))
	for name, version := range parsed.Dependencies {
		if isUnsupportedNodeDependencyVersion(version) {
			continue
		}
		if version == "" {
			specs = append(specs, name)
		} else {
			specs = append(specs, name+"@"+version)
		}
	}
	sort.Strings(specs)
	return specs, nil
}

func isUnsupportedNodeDependencyVersion(version string) bool {
	version = strings.TrimSpace(strings.ToLower(version))
	for _, prefix := range []string{"file:", "link:", "workspace:", "git+", "http:", "https:"} {
		if strings.HasPrefix(version, prefix) {
			return true
		}
	}
	return false
}

var pythonImportRE = regexp.MustCompile(`(?m)^\s*(?:from\s+([A-Za-z_][A-Za-z0-9_\.]*)\s+import|import\s+([A-Za-z_][A-Za-z0-9_\.]*))`)
var nodeImportRE = regexp.MustCompile(`(?m)(?:import\s+(?:[^'"]+\s+from\s+)?|import\s*\(|require\s*\()\s*['"]([^'"]+)['"]`)

func scanPythonImports(file *zip.File) ([]string, error) {
	content, err := readZipFile(file, 256*1024)
	if err != nil {
		return nil, err
	}
	matches := pythonImportRE.FindAllStringSubmatch(string(content), -1)
	specSet := map[string]struct{}{}
	for _, match := range matches {
		module := match[1]
		if module == "" {
			module = match[2]
		}
		module = strings.Split(module, ".")[0]
		if module == "" || pythonStdlibModules[module] {
			continue
		}
		specSet[normalizePackageName("python3", module)] = struct{}{}
	}
	return sortedSourceSet(specSet), nil
}

func scanNodeImports(file *zip.File) ([]string, error) {
	content, err := readZipFile(file, 256*1024)
	if err != nil {
		return nil, err
	}
	matches := nodeImportRE.FindAllStringSubmatch(string(content), -1)
	specSet := map[string]struct{}{}
	for _, match := range matches {
		name := strings.TrimSpace(match[1])
		if name == "" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "/") || nodeBuiltinModules[name] {
			continue
		}
		if strings.HasPrefix(name, "@") {
			parts := strings.Split(name, "/")
			if len(parts) >= 2 {
				name = parts[0] + "/" + parts[1]
			}
		} else {
			name = strings.Split(name, "/")[0]
		}
		specSet[normalizePackageName("nodejs", name)] = struct{}{}
	}
	return sortedSourceSet(specSet), nil
}

func inferPrimaryLanguage(files map[string]*zip.File) string {
	python := 0
	node := 0
	for path := range files {
		if strings.HasPrefix(path, "scripts/") {
			switch filepath.Ext(path) {
			case ".py":
				python++
			case ".js", ".mjs", ".cjs":
				node++
			}
		}
	}
	if node > python {
		return "nodejs"
	}
	if python > 0 {
		return "python3"
	}
	return ""
}

func dependencyFingerprint(request DependencyRequest) string {
	normalized := request
	normalized.Python = stripDependencySources(normalized.Python)
	normalized.Node = stripDependencySources(normalized.Node)
	normalized.System = stripDependencySources(normalized.System)
	raw, _ := json.Marshal(normalized)
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func stripDependencySources(items []DetectedDependency) []DetectedDependency {
	result := append([]DetectedDependency(nil), items...)
	for i := range result {
		result[i].Source = nil
	}
	return result
}

func sortedDependencies(packages map[string]DetectedDependency) []DetectedDependency {
	items := make([]DetectedDependency, 0, len(packages))
	for _, item := range packages {
		sort.Strings(item.Source)
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Ecosystem != items[j].Ecosystem {
			return items[i].Ecosystem < items[j].Ecosystem
		}
		if items[i].Name != items[j].Name {
			return items[i].Name < items[j].Name
		}
		return items[i].Version < items[j].Version
	})
	return items
}

func sortedSourceSet(set map[string]struct{}) []string {
	items := make([]string, 0, len(set))
	for item := range set {
		if item != "" {
			items = append(items, item)
		}
	}
	sort.Strings(items)
	return items
}

func appendUniqueString(items []string, value string) []string {
	if value == "" {
		return items
	}
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

var pythonImportPackageMap = map[string]string{
	"PIL":        "Pillow",
	"Image":      "Pillow",
	"cv2":        "opencv-python-headless",
	"sklearn":    "scikit-learn",
	"yaml":       "PyYAML",
	"bs4":        "beautifulsoup4",
	"fitz":       "PyMuPDF",
	"docx":       "python-docx",
	"pptx":       "python-pptx",
	"pandas":     "pandas",
	"numpy":      "numpy",
	"openpyxl":   "openpyxl",
	"pdfplumber": "pdfplumber",
}

var pythonStdlibModules = map[string]bool{
	"argparse": true, "base64": true, "collections": true, "contextlib": true,
	"csv": true, "datetime": true, "decimal": true, "functools": true,
	"hashlib": true, "io": true, "itertools": true, "json": true,
	"logging": true, "math": true, "os": true, "pathlib": true, "random": true,
	"re": true, "shutil": true, "statistics": true, "string": true,
	"subprocess": true, "sys": true, "tempfile": true, "time": true,
	"typing": true, "uuid": true, "zipfile": true,
}

var nodeBuiltinModules = map[string]bool{
	"assert": true, "buffer": true, "child_process": true, "crypto": true,
	"events": true, "fs": true, "http": true, "https": true, "net": true,
	"os": true, "path": true, "process": true, "stream": true, "url": true,
	"util": true, "zlib": true,
}
