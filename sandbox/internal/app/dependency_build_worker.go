package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/executor"
	"github.com/zgiai/zgi-sandbox/internal/observer"
	"github.com/zgiai/zgi-sandbox/internal/policy"
	"github.com/zgiai/zgi-sandbox/internal/runner"
	"github.com/zgiai/zgi-sandbox/internal/storage"
)

type dependencyBuildWorkerInput struct {
	BuildID           string                        `json:"build_id"`
	Fingerprint       string                        `json:"fingerprint"`
	ProfileName       string                        `json:"profile_name"`
	DependencyRequest executor.DependencyRequest    `json:"dependency_request"`
	Packages          []executor.DetectedDependency `json:"packages"`
	OutputDir         string                        `json:"output_dir"`
	RuntimeRootFSDir  string                        `json:"runtime_rootfs_dir"`
	RootFSDir         string                        `json:"rootfs_dir"`
}

func (s *Server) handleDependencyBuildRun(w http.ResponseWriter, r *http.Request, fingerprint string) {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		writeEnvelopeWithMessage(w, http.StatusBadRequest, -400, "fingerprint is required", nil)
		return
	}
	record, err := s.store.GetDependencyBuildRequest(fingerprint)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	if record.Status == "ready" {
		response, err := dependencyBuildResponseFromRecord(record)
		if err != nil {
			writeKnownError(w, err)
			return
		}
		response.OrganizationID = requestOrganizationID(r, "")
		writeEnvelope(w, http.StatusOK, response)
		return
	}
	if strings.TrimSpace(s.config.DependencyBuildCommand) == "" {
		writeEnvelopeWithMessage(w, http.StatusBadRequest, -400, "dependency build command is not configured", nil)
		return
	}
	if strings.TrimSpace(s.config.DependencyRootFSDir) == "" {
		writeEnvelopeWithMessage(w, http.StatusBadRequest, -400, "dependency rootfs directory is not configured", nil)
		return
	}

	record, err = s.store.UpdateDependencyBuildRequestStatus(record.Fingerprint, "building", record.ArtifactChecksum, record.SizeBytes, "")
	if err != nil {
		writeKnownError(w, err)
		return
	}
	s.observer.Record("dependency_build.building", "", "dependency build started", observer.MetadataWithContext(r.Context(), dependencyBuildEventMetadata(record)))

	record, err = s.runDependencyBuildCommand(r.Context(), record)
	if err != nil {
		failed, updateErr := s.store.UpdateDependencyBuildRequestStatus(fingerprint, "failed", "", 0, err.Error())
		if updateErr == nil {
			record = failed
		}
		metadata := dependencyBuildEventMetadata(record)
		metadata["error"] = err.Error()
		s.observer.Record("dependency_build.failed", "", "dependency build failed", observer.MetadataWithContext(r.Context(), metadata))
		response, decodeErr := dependencyBuildResponseFromRecord(record)
		if decodeErr != nil {
			writeKnownError(w, decodeErr)
			return
		}
		response.OrganizationID = requestOrganizationID(r, "")
		writeEnvelopeWithMessage(w, http.StatusBadRequest, -400, "dependency build failed", response)
		return
	}

	response, err := dependencyBuildResponseFromRecord(record)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	response.OrganizationID = requestOrganizationID(r, "")
	s.observer.Record("dependency_build.ready", "", "dependency build completed", observer.MetadataWithContext(r.Context(), dependencyBuildEventMetadata(record)))
	writeEnvelope(w, http.StatusOK, response)
}

func (s *Server) runDependencyBuildCommand(ctx context.Context, record *storage.DependencyBuildRequestRecord) (*storage.DependencyBuildRequestRecord, error) {
	buildInput, err := dependencyBuildWorkerInputFromRecord(record, s.config.DependencyRootFSDir)
	if err != nil {
		return record, err
	}
	cleanupArtifact := true
	defer func() {
		if cleanupArtifact {
			_ = os.RemoveAll(buildInput.RuntimeRootFSDir)
		}
	}()
	if err := os.RemoveAll(buildInput.RuntimeRootFSDir); err != nil {
		return record, fmt.Errorf("clean dependency build output root: %w", err)
	}
	if err := os.MkdirAll(buildInput.OutputDir, 0o755); err != nil {
		return record, fmt.Errorf("prepare dependency build output directory: %w", err)
	}
	inputPath, err := s.writeDependencyBuildInput(buildInput)
	if err != nil {
		return record, err
	}
	defer func() {
		_ = os.Remove(inputPath)
	}()

	command := strings.Fields(s.config.DependencyBuildCommand)
	if len(command) == 0 {
		return record, errors.New("dependency build command is empty")
	}
	timeout := time.Duration(s.config.DependencyProfileBuildTimeoutSeconds) * time.Second
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, command[0], append(command[1:], inputPath, buildInput.OutputDir)...)
	cmd.Env = append(os.Environ(),
		"ZGI_DEPENDENCY_BUILD_INPUT="+inputPath,
		"ZGI_DEPENDENCY_BUILD_OUTPUT_DIR="+buildInput.OutputDir,
		"ZGI_DEPENDENCY_BUILD_ROOTFS_DIR="+buildInput.RootFSDir,
		"ZGI_DEPENDENCY_BUILD_PROFILE_NAME="+buildInput.ProfileName,
		"ZGI_DEPENDENCY_BUILD_FINGERPRINT="+buildInput.Fingerprint,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return record, fmt.Errorf("dependency build command timed out after %s", timeout)
		}
		return record, fmt.Errorf("dependency build command failed: %w: %s", err, truncateCommandOutput(string(output), 2048))
	}

	artifact, err := s.findDependencyBuildArtifact(record.ProfileName)
	if err != nil {
		return record, err
	}
	profile, err := s.dependencyBuildProfile(record, artifact)
	if err != nil {
		return record, err
	}
	result := policy.DependencyProfileBuildResult{
		BuildID:  record.BuildID,
		Accepted: true,
		Status:   "ready",
		Profile:  &profile,
	}
	if _, err := s.policy.RegisterDependencyProfile(profile, result); err != nil {
		return record, err
	}
	if err := s.store.SaveDependencyProfile(profile); err != nil {
		s.policy.RemoveDependencyProfileRef(profile)
		return record, err
	}
	updated, err := s.store.UpdateDependencyBuildRequestStatus(record.Fingerprint, "ready", artifact.Checksum, artifact.SizeBytes, "")
	if err != nil {
		s.policy.RemoveDependencyProfileRef(profile)
		return record, err
	}
	cleanupArtifact = false
	return updated, nil
}

func (s *Server) refreshStaleDependencyBuildRecord(record *storage.DependencyBuildRequestRecord) (*storage.DependencyBuildRequestRecord, error) {
	if record == nil || record.Status != "ready" || strings.TrimSpace(record.ArtifactChecksum) == "" {
		return record, nil
	}
	available, err := s.dependencyBuildArtifactAvailable(record.ArtifactChecksum)
	if err != nil {
		return record, err
	}
	if available {
		return record, nil
	}

	s.policy.RemoveDependencyProfileRef(policy.DependencyProfile{
		Name:  record.ProfileName,
		Scope: "global",
	})
	return s.store.UpdateDependencyBuildRequestStatus(record.Fingerprint, "queued", "", 0, "")
}

func (s *Server) dependencyBuildArtifactAvailable(artifactChecksum string) (bool, error) {
	artifactChecksum = strings.TrimSpace(artifactChecksum)
	if artifactChecksum == "" || strings.TrimSpace(s.config.DependencyRootFSDir) == "" {
		return true, nil
	}
	artifacts, err := runner.ListDependencyProfileArtifacts(s.config.DependencyRootFSDir)
	if err != nil {
		return false, err
	}
	for _, artifact := range artifacts {
		if artifact.Checksum == artifactChecksum {
			return true, nil
		}
	}
	return false, nil
}

func dependencyBuildWorkerInputFromRecord(record *storage.DependencyBuildRequestRecord, root string) (dependencyBuildWorkerInput, error) {
	if record == nil {
		return dependencyBuildWorkerInput{}, errors.New("dependency build request not found")
	}
	var request executor.DependencyRequest
	if err := json.Unmarshal(record.DependencyRequestJSON, &request); err != nil {
		return dependencyBuildWorkerInput{}, fmt.Errorf("decode dependency request: %w", err)
	}
	var packages []executor.DetectedDependency
	if err := json.Unmarshal(record.PackagesJSON, &packages); err != nil {
		return dependencyBuildWorkerInput{}, fmt.Errorf("decode dependency packages: %w", err)
	}
	profileName := strings.TrimSpace(record.ProfileName)
	if !safeAppDependencyProfileName(profileName) {
		return dependencyBuildWorkerInput{}, fmt.Errorf("dependency build profile name is not safe: %s", profileName)
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return dependencyBuildWorkerInput{}, errors.New("dependency rootfs directory is required")
	}
	runtimeRootFSDir := filepath.Join(root, profileName)
	outputDir := filepath.Join(runtimeRootFSDir, strings.TrimPrefix(secureProfileBasePathForApp(), "/"), profileName)
	return dependencyBuildWorkerInput{
		BuildID:           record.BuildID,
		Fingerprint:       record.Fingerprint,
		ProfileName:       profileName,
		DependencyRequest: request,
		Packages:          packages,
		OutputDir:         outputDir,
		RuntimeRootFSDir:  runtimeRootFSDir,
		RootFSDir:         root,
	}, nil
}

func (s *Server) writeDependencyBuildInput(input dependencyBuildWorkerInput) (string, error) {
	dir := filepath.Join(s.config.DataDir, "dependency-build-inputs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("prepare dependency build input directory: %w", err)
	}
	file, err := os.CreateTemp(dir, input.BuildID+"-*.json")
	if err != nil {
		return "", fmt.Errorf("create dependency build input: %w", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(input)
	closeErr := file.Close()
	if encodeErr != nil {
		_ = os.Remove(file.Name())
		return "", fmt.Errorf("write dependency build input: %w", encodeErr)
	}
	if closeErr != nil {
		_ = os.Remove(file.Name())
		return "", fmt.Errorf("close dependency build input: %w", closeErr)
	}
	return file.Name(), nil
}

func (s *Server) findDependencyBuildArtifact(profileName string) (runner.DependencyProfileArtifact, error) {
	artifacts, err := runner.ListDependencyProfileArtifacts(s.config.DependencyRootFSDir)
	if err != nil {
		return runner.DependencyProfileArtifact{}, err
	}
	for _, artifact := range artifacts {
		if artifact.Name == profileName {
			return artifact, nil
		}
	}
	return runner.DependencyProfileArtifact{}, fmt.Errorf("dependency build artifact is missing for profile %s", profileName)
}

func (s *Server) dependencyBuildProfile(record *storage.DependencyBuildRequestRecord, artifact runner.DependencyProfileArtifact) (policy.DependencyProfile, error) {
	var packages []executor.DetectedDependency
	if err := json.Unmarshal(record.PackagesJSON, &packages); err != nil {
		return policy.DependencyProfile{}, fmt.Errorf("decode dependency packages: %w", err)
	}
	var request executor.DependencyRequest
	if err := json.Unmarshal(record.DependencyRequestJSON, &request); err != nil {
		return policy.DependencyProfile{}, fmt.Errorf("decode dependency request: %w", err)
	}
	policyPackages := make([]policy.DependencyPackage, 0, len(packages))
	for _, item := range packages {
		policyPackages = append(policyPackages, policy.DependencyPackage{
			Ecosystem: item.Ecosystem,
			Name:      item.Name,
			Version:   defaultDependencyVersion(item.Version),
		})
	}
	return policy.DependencyProfile{
		Name:             record.ProfileName,
		Version:          dependencyBuildProfileVersion(record.Fingerprint),
		Status:           "ready",
		Enabled:          true,
		OwnerScope:       "global",
		Scope:            "global",
		Languages:        dependencyBuildLanguages(request, packages, artifact.Languages),
		Packages:         policyPackages,
		BaseRuntime:      defaultNonEmptyString(request.BaseRuntime, artifact.BaseRuntime),
		Checksum:         record.Fingerprint,
		ArtifactChecksum: artifact.Checksum,
		SizeBytes:        artifact.SizeBytes,
		Description:      "Automatically built dependency profile.",
		PublicReusable:   true,
		Pinned:           true,
	}, nil
}

func dependencyBuildLanguages(request executor.DependencyRequest, packages []executor.DetectedDependency, artifactLanguages []string) []string {
	seen := map[string]bool{}
	var values []string
	add := func(language string) {
		language = strings.TrimSpace(language)
		if language == "" || seen[language] {
			return
		}
		switch language {
		case "python", "python3":
			language = "python3"
		case "node", "nodejs", "javascript":
			language = "nodejs"
		default:
			return
		}
		if seen[language] {
			return
		}
		seen[language] = true
		values = append(values, language)
	}
	add(request.Language)
	for _, item := range packages {
		switch item.Ecosystem {
		case "python3", "nodejs":
			add(item.Ecosystem)
		}
	}
	for _, language := range artifactLanguages {
		add(language)
	}
	if len(values) == 0 {
		values = append(values, "python3")
	}
	return values
}

func dependencyBuildProfileVersion(fingerprint string) string {
	return "sha256-" + dependencyFingerprintSuffix(fingerprint, 16)
}

func defaultDependencyVersion(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "managed"
	}
	return strings.TrimPrefix(value, "==")
}

func defaultNonEmptyString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return strings.TrimSpace(fallback)
	}
	return value
}

func dependencyBuildEventMetadata(record *storage.DependencyBuildRequestRecord) map[string]any {
	metadata := map[string]any{}
	if record == nil {
		return metadata
	}
	metadata["build_id"] = record.BuildID
	metadata["fingerprint"] = record.Fingerprint
	metadata["status"] = record.Status
	metadata["profile_name"] = record.ProfileName
	metadata["package_count"] = record.PackageCount
	if record.ArtifactChecksum != "" {
		metadata["artifact_checksum"] = record.ArtifactChecksum
	}
	if record.SizeBytes > 0 {
		metadata["size_bytes"] = record.SizeBytes
	}
	return metadata
}

func truncateCommandOutput(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "...truncated"
}

func secureProfileBasePathForApp() string {
	return "/opt/zgi/profiles"
}

func safeAppDependencyProfileName(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' ||
			char >= '0' && char <= '9' ||
			char == '-' {
			continue
		}
		return false
	}
	return true
}
