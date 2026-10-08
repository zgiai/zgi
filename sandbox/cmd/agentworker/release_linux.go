//go:build linux

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func decodeAdmissionLimits(limits *agentexecution.AdmissionLimits) error {
	// Conservative defaults for small, dedicated test hosts. These are backend
	// totals shared across processes, not guesses about the host's free memory.
	values := []struct {
		name     string
		target   *int
		fallback int
	}{
		{"MAX_BINDINGS", &limits.Bindings, 2},
		{"MAX_BINDINGS_PER_ORG", &limits.BindingsPerOrg, 2},
		{"MAX_JOBS", &limits.Jobs, 2},
		{"MAX_JOBS_PER_ORG", &limits.JobsPerOrg, 2},
		{"MAX_JOBS_PER_BINDING", &limits.JobsPerBinding, 1},
		{"JOB_TIMEOUT_SECONDS", &limits.JobTimeoutSeconds, 300},
	}
	for _, value := range values {
		name := "ZGI_AGENT_RUNTIME_" + value.name
		text := strings.TrimSpace(os.Getenv(name))
		*value.target = value.fallback
		if text != "" {
			parsed, err := strconv.Atoi(text)
			if err != nil || parsed < 1 {
				return fmt.Errorf("%s must be a positive integer", name)
			}
			*value.target = parsed
		}
	}
	return limits.Validate()
}

func workerReleaseFingerprint(cfg workerConfig) (worker, configuration string, err error) {
	// Only effective execution configuration is hashed. Never hash credentials,
	// DB URLs, per-process identifiers, listen addresses or storage credentials.
	data, err := json.Marshal(struct {
		Backend           string
		Profiles          any
		PlatformCLI       bool
		Admission         agentexecution.AdmissionLimits
		MemoryBytes       int64
		PIDs              int64
		CPUPeriod         uint64
		CPUQuota          int64
		IdleTTLSeconds    int64
		E2BTimeoutSeconds int
	}{
		Backend: cfg.Backend, Profiles: releaseProfiles(cfg), PlatformCLI: cfg.PlatformCLI,
		Admission: cfg.Admission, MemoryBytes: cfg.MemoryBytes, PIDs: cfg.PIDs,
		CPUPeriod: cfg.CPUPeriod, CPUQuota: cfg.CPUQuota,
		IdleTTLSeconds: int64(cfg.IdleTTL.Seconds()), E2BTimeoutSeconds: cfg.E2BTimeoutSeconds,
	})
	if err != nil {
		return "", "", err
	}
	configuration, err = agentexecution.ReleaseDigest(bytes.NewReader(data))
	if err != nil {
		return "", "", err
	}
	// Hash the running inode, not a pathname that an atomic binary upgrade may
	// already have replaced while this process is starting.
	file, err := os.Open("/proc/self/exe")
	if err != nil {
		return "", "", err
	}
	defer file.Close()
	worker, err = agentexecution.ReleaseDigest(file)
	return worker, configuration, err
}

func releaseProfiles(cfg workerConfig) any {
	if cfg.Backend == e2bBackendName {
		return cfg.E2BProfiles
	}
	return cfg.Profiles
}

func printReleaseFingerprint(cfg workerConfig) error {
	worker, configuration, err := workerReleaseFingerprint(cfg)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"worker_digest": worker, "configuration_digest": configuration,
		"platform_protocol_version": agentexecution.PlatformProtocolVersion,
		"version":                   agentexecution.ReleaseContractVersion,
	})
}

func verifyWorkerRelease(cfg workerConfig) (string, error) {
	// Existing installations remain compatible until operators opt in with a
	// manifest. Once configured, missing files or mismatches NEVER fall back.
	if cfg.ReleaseManifestPath == "" {
		if cfg.ReleaseRequired {
			return "", fmt.Errorf("ZGI_AGENT_RUNTIME_RELEASE_MANIFEST is required for strict releases")
		}
		return "", nil
	}
	info, err := os.Lstat(cfg.ReleaseManifestPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 65536 || info.Mode().Perm()&0022 != 0 {
		return "", fmt.Errorf("release manifest must be a regular, non-world/group-writable file of at most 64 KiB")
	}
	file, err := os.Open(cfg.ReleaseManifestPath)
	if err != nil {
		return "", fmt.Errorf("open runtime release manifest: %w", err)
	}
	defer file.Close()
	worker, configuration, err := workerReleaseFingerprint(cfg)
	if err != nil {
		return "", err
	}
	manifest, err := agentexecution.VerifyRelease(file, worker, configuration)
	if err != nil {
		return "", err
	}
	return manifest.ID, nil
}
