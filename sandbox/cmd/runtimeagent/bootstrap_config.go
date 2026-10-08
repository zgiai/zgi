package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"

	"github.com/zgiai/zgi-sandbox/internal/packagesource"
)

func runtimeBootstrapConfig(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 16*1024 {
		return errors.New("invalid runtime bootstrap configuration file")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 16*1024+1))
	decoder.DisallowUnknownFields()
	var config packagesource.Bootstrap
	if err := decoder.Decode(&config); err != nil {
		return errors.New("invalid runtime bootstrap configuration")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("multiple runtime bootstrap configurations")
	}
	if err := config.Sources.Validate(config.AllowedHosts); err != nil {
		return err
	}
	if config.Runtime != nil {
		if err := applyBootstrapRuntime(config.Runtime, config.PlatformCLI); err != nil {
			return err
		}
	}
	hosts, err := json.Marshal(config.AllowedHosts)
	if err != nil {
		return err
	}
	hostEnvironment := string(hosts)
	if len(config.AllowedHosts) == 0 {
		hostEnvironment = ""
	}
	for key, value := range map[string]string{
		"ZGI_RUNTIME_AGENT_PLATFORM_CLI_ENABLED":      map[bool]string{true: "true", false: "false"}[config.PlatformCLI],
		"ZGI_RUNTIME_AGENT_PACKAGE_SOURCES_JSON":      config.Sources.JSON(),
		"ZGI_RUNTIME_AGENT_EGRESS_ALLOWED_HOSTS_JSON": hostEnvironment,
		"ZGI_RUNTIME_AGENT_PROFILE_IDENTITY":          config.ProfileIdentity,
		"ZGI_RUNTIME_AGENT_PACKAGE_SOURCE_CHECK":      map[bool]string{true: "true", false: "false"}[config.Check],
	} {
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return os.Remove(path)
}

func applyBootstrapRuntime(policy *packagesource.RuntimeConfig, platform bool) error {
	if (policy.NetworkMode != "disabled" && policy.NetworkMode != "package-install") ||
		(policy.NetworkMode == "disabled" && (policy.EgressMode != "bridge" || policy.DependencyInstallation || policy.PackageNetworkMode != "")) ||
		(policy.NetworkMode == "package-install" && (policy.EgressMode != "direct" || !policy.DependencyInstallation || policy.PackageNetworkMode != "package-install")) ||
		((platform || policy.DependencyInstallation) && (policy.JobUID == 0 || policy.JobGID == 0)) {
		return errors.New("invalid runtime bootstrap policy")
	}
	for key, value := range map[string]string{
		"ZGI_RUNTIME_AGENT_NETWORK_MODE":                    policy.NetworkMode,
		"ZGI_RUNTIME_AGENT_EGRESS_MODE":                     policy.EgressMode,
		"ZGI_RUNTIME_AGENT_DEPENDENCY_INSTALLATION_ENABLED": strconv.FormatBool(policy.DependencyInstallation),
		"ZGI_RUNTIME_AGENT_PACKAGE_NETWORK_MODE":            policy.PackageNetworkMode,
		"ZGI_RUNTIME_AGENT_DEPENDENCY_PREPARATION_ENABLED":  strconv.FormatBool(policy.DependencyPreparation),
		"ZGI_RUNTIME_AGENT_DEPENDENCY_PREPARATION_MODE":     policy.DependencyPreparationMode,
		"ZGI_RUNTIME_AGENT_JOB_UID":                         strconv.FormatUint(uint64(policy.JobUID), 10),
		"ZGI_RUNTIME_AGENT_JOB_GID":                         strconv.FormatUint(uint64(policy.JobGID), 10),
	} {
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return nil
}
