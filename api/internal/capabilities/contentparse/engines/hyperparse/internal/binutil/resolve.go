package binutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func Resolve(command string, configuredPath string) (string, error) {
	configuredPath = strings.TrimSpace(configuredPath)
	if configuredPath != "" {
		if _, err := os.Stat(configuredPath); err == nil {
			return configuredPath, nil
		}
		return "", fmt.Errorf("configured binary not found: %s", configuredPath)
	}
	return ResolveCommand(command)
}

func ResolveCommand(command string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", fmt.Errorf("empty binary command")
	}
	if strings.ContainsAny(command, `/\`) {
		if _, err := os.Stat(command); err != nil {
			return "", fmt.Errorf("binary command not found: %s", command)
		}
		return command, nil
	}
	for _, candidate := range Candidates(command) {
		if p, err := exec.LookPath(candidate); err == nil && strings.TrimSpace(p) != "" {
			return p, nil
		}
		if filepath.IsAbs(candidate) {
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("binary_not_found(%s)", command)
}

func Candidates(name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	candidates := []string{name}
	if runtime.GOOS == "windows" {
		return append(candidates, name+".exe")
	}
	for _, dir := range []string{
		"/opt/homebrew/bin",
		"/usr/local/bin",
		"/usr/bin",
		"/bin",
		"/opt/local/bin",
	} {
		candidates = append(candidates, filepath.Join(dir, name))
	}
	return candidates
}
