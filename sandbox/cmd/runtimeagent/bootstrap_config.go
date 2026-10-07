package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"

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
	hosts, err := json.Marshal(config.AllowedHosts)
	if err != nil {
		return err
	}
	for key, value := range map[string]string{
		"ZGI_RUNTIME_AGENT_PACKAGE_SOURCES_JSON":      config.Sources.JSON(),
		"ZGI_RUNTIME_AGENT_EGRESS_ALLOWED_HOSTS_JSON": string(hosts),
		"ZGI_RUNTIME_AGENT_PROFILE_IDENTITY":          config.ProfileIdentity,
		"ZGI_RUNTIME_AGENT_PACKAGE_SOURCE_CHECK":      map[bool]string{true: "true", false: "false"}[config.Check],
	} {
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return os.Remove(path)
}
