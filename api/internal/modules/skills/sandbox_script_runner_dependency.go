package skills

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (r *SandboxScriptRunner) preflightDependencyProfile(ctx context.Context, execCtx ExecutionContext, language string, dependencyProfile string) error {
	profile := strings.TrimSpace(dependencyProfile)
	if profile == "" {
		profile = defaultSkillDependencyProfile
	}
	language = normalizeSkillScriptLanguage(language)
	if language == "" {
		language = "python3"
	}

	var catalog sandboxDependencyCatalog
	endpoint := "/v1/sandbox/dependencies?language=" + url.QueryEscape(language)
	endpoint = withOrganizationQuery(endpoint, execCtx)
	if err := r.doIdempotentJSON(ctx, http.MethodGet, endpoint, nil, &catalog, r.timeouts.Create); err != nil {
		return fmt.Errorf("skill dependency profile preflight failed: %w", err)
	}

	for _, item := range catalog.Profiles {
		if item.Name != profile {
			continue
		}
		if !item.Enabled || item.Status != "ready" {
			return fmt.Errorf("skill dependency profile preflight failed: dependency profile is not ready: %s", profile)
		}
		return nil
	}
	return fmt.Errorf("skill dependency profile preflight failed: unsupported dependency profile for %s: %s", language, profile)
}

func (r *SandboxScriptRunner) prepareDependencyProfile(ctx context.Context, execCtx ExecutionContext, archiveBase64 string, language string) (string, error) {
	var build sandboxDependencyBuild
	payload := map[string]interface{}{
		"archive_base64": archiveBase64,
		"format":         "zip",
		"strip_root":     false,
		"language":       normalizeSkillScriptLanguage(language),
	}
	if organizationID := strings.TrimSpace(execCtx.OrganizationID); organizationID != "" {
		payload["organization_id"] = organizationID
	}
	if err := r.doJSON(ctx, http.MethodPost, withOrganizationQuery("/v1/sandbox/dependencies/builds", execCtx), payload, &build, r.timeouts.Create); err != nil {
		return "", fmt.Errorf("skill dependency prepare failed: %w", err)
	}
	return r.waitDependencyBuildReady(ctx, execCtx, build)
}

func (r *SandboxScriptRunner) waitDependencyBuildReady(ctx context.Context, execCtx ExecutionContext, build sandboxDependencyBuild) (string, error) {
	if strings.TrimSpace(build.ProfileName) == "" {
		return "", fmt.Errorf("skill dependency prepare failed: sandbox did not return a dependency profile")
	}
	if build.Status == "ready" {
		return build.ProfileName, nil
	}
	if build.Status == "failed" {
		return "", fmt.Errorf("skill dependency build failed: %s", strings.TrimSpace(build.Error))
	}
	if strings.TrimSpace(build.Fingerprint) == "" {
		return "", fmt.Errorf("skill dependency build did not return a fingerprint")
	}

	timeout := r.timeouts.DependencyBuild
	if timeout <= 0 {
		timeout = defaultSandboxDependencyBuildTimeout
	}
	pollInterval := r.timeouts.DependencyBuildPoll
	if pollInterval <= 0 {
		pollInterval = defaultSandboxDependencyBuildPollInterval
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	timer := time.NewTimer(pollInterval)
	defer timer.Stop()

	for {
		select {
		case <-waitCtx.Done():
			return "", fmt.Errorf("skill dependency build timed out after %s for %s", timeout, build.ProfileName)
		case <-timer.C:
			var current sandboxDependencyBuild
			endpoint := "/v1/sandbox/dependencies/builds/" + url.PathEscape(build.Fingerprint)
			endpoint = withOrganizationQuery(endpoint, execCtx)
			if err := r.doIdempotentJSON(waitCtx, http.MethodGet, endpoint, nil, &current, r.timeouts.Create); err != nil {
				return "", fmt.Errorf("skill dependency build status failed: %w", err)
			}
			switch current.Status {
			case "ready":
				if strings.TrimSpace(current.ProfileName) == "" {
					return "", fmt.Errorf("skill dependency build ready without profile name")
				}
				return current.ProfileName, nil
			case "failed":
				return "", fmt.Errorf("skill dependency build failed: %s", strings.TrimSpace(current.Error))
			default:
				timer.Reset(pollInterval)
			}
		}
	}
}
