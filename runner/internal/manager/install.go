package manager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"plugin_runner/internal/config"
	"plugin_runner/internal/plugin"
)

func (m *Manager) installDependencies(ctx context.Context, manifest plugin.Manifest, workdir string) error {
	pkgs := manifest.Requirements.Packages
	switch manifest.Runner.Language {
	case plugin.LanguagePython:
		return m.setupPythonEnv(ctx, workdir)
	case plugin.LanguageNode:
		return m.setupNodeEnv(ctx, workdir, pkgs)
	default:
		return fmt.Errorf("dependency install not supported for language %q", manifest.Runner.Language)
	}
}

func (m *Manager) setupPythonEnv(ctx context.Context, workdir string) error {
	venvPath := filepath.Join(workdir, ".venv")
	pythonPath := filepath.Join(venvPath, "bin", "python")
	if exists, _ := fileExists(pythonPath); !exists {
		if err := m.createVenv(ctx, workdir, venvPath); err != nil {
			return err
		}
	}

	reqPath := filepath.Join(workdir, "requirements.txt")
	if exists, _ := fileExists(reqPath); !exists {
		return fmt.Errorf("requirements.txt not found in %s", workdir)
	}

	if err := m.installPythonDeps(ctx, workdir, venvPath, reqPath); err != nil {
		return err
	}

	if err := m.precompile(ctx, pythonPath, workdir); err != nil {
		m.log.Warn("precompile failed", zap.Error(err))
	}
	return nil
}

// ParseRequirementsTxt parses a requirements.txt file and returns the package list.
func ParseRequirementsTxt(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var packages []string
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Skip -r, -e, --extra-index-url, etc.
		if strings.HasPrefix(line, "-") {
			continue
		}
		packages = append(packages, line)
	}
	return packages, nil
}

func (m *Manager) createVenv(ctx context.Context, workdir, venvPath string) error {
	ctx, cancel := context.WithTimeout(ctx, m.cfg.PythonEnvInitTimeout)
	defer cancel()

	uv := strings.TrimSpace(m.cfg.UVPath)
	if uv == "" {
		uv = m.findUV(ctx, workdir)
	}

	var cmd *exec.Cmd
	if uv != "" {
		cmd = exec.CommandContext(ctx, uv, "venv", ".venv", "--python", m.cfg.PythonInterpreter)
	} else {
		cmd = exec.CommandContext(ctx, m.cfg.PythonInterpreter, "-m", "venv", ".venv")
	}
	cmd.Dir = workdir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create venv failed: %w, output: %s", err, string(output))
	}
	return nil
}

func (m *Manager) findUV(ctx context.Context, workdir string) string {
	// First, try to find uv in PATH directly
	if path, err := exec.LookPath("uv"); err == nil {
		return path
	}

	// Fallback: try to find via Python uv package (for compatibility)
	cmd := exec.CommandContext(ctx, m.cfg.PythonInterpreter, "-c", "from uv._find_uv import find_uv_bin; print(find_uv_bin())")
	cmd.Dir = workdir
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func (m *Manager) installPythonDeps(ctx context.Context, workdir, venvPath, reqPath string) error {
	pipArgs := []string{"install", "-r", reqPath}
	if m.cfg.PipMirrorURL != "" {
		pipArgs = append(pipArgs, "-i", m.cfg.PipMirrorURL)
	}
	if m.cfg.PipVerbose {
		pipArgs = append(pipArgs, "-vvv")
	}
	if strings.TrimSpace(m.cfg.PipExtraArgs) != "" {
		pipArgs = append(pipArgs, strings.Fields(m.cfg.PipExtraArgs)...)
	}

	env := []string{
		fmt.Sprintf("VIRTUAL_ENV=%s", venvPath),
		fmt.Sprintf("PATH=%s", filepath.Join(venvPath, "bin")+":"+os.Getenv("PATH")),
	}

	uv := strings.TrimSpace(m.cfg.UVPath)
	if uv == "" {
		uv = m.findUV(ctx, workdir)
	}

	installCmd := filepath.Join(venvPath, "bin", "pip")
	installArgs := append([]string{}, pipArgs...)
	if uv != "" {
		// The venv was created with the correct Python version (--python in createVenv)
		// No need to specify --python in pip install, VIRTUAL_ENV env var handles this
		installCmd = uv
		installArgs = append([]string{"pip"}, pipArgs...)
	} else if m.cfg.PipPreferBinary {
		// uv pip does not support this flag; keep it only for direct pip path.
		installArgs = append(installArgs, "--prefer-binary")
	}

	return m.runInstallWithPreferBinaryFallback(ctx, installCmd, installArgs, workdir, env)
}

func (m *Manager) runInstallWithPreferBinaryFallback(ctx context.Context, cmd string, args []string, workdir string, env []string) error {
	if err := m.runInstallCmd(ctx, cmd, args, workdir, env); err != nil {
		if !containsArg(args, "--prefer-binary") || !isPreferBinaryUnsupportedError(err) {
			return err
		}

		retryArgs := removeFirstArg(args, "--prefer-binary")
		m.log.Warn("installer does not support --prefer-binary, retrying without it",
			zap.String("cmd", cmd),
			zap.Strings("args", args),
			zap.Error(err),
		)
		return m.runInstallCmd(ctx, cmd, retryArgs, workdir, env)
	}
	return nil
}

func containsArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}

func removeFirstArg(args []string, target string) []string {
	result := make([]string, 0, len(args))
	removed := false
	for _, arg := range args {
		if !removed && arg == target {
			removed = true
			continue
		}
		result = append(result, arg)
	}
	return result
}

func isPreferBinaryUnsupportedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	patterns := []string{
		"no such option: --prefer-binary",
		"unknown option --prefer-binary",
		"unknown option: --prefer-binary",
		"unrecognized arguments: --prefer-binary",
		"option --prefer-binary not recognized",
	}
	for _, pattern := range patterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
}

func (m *Manager) setupNodeEnv(ctx context.Context, workdir string, packages []string) error {
	if len(packages) == 0 {
		return nil
	}
	envDir := filepath.Join(workdir, ".node_env")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		return fmt.Errorf("create node env: %w", err)
	}

	if err := m.writePackageJSON(envDir, packages); err != nil {
		return err
	}

	env := []string{
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
		fmt.Sprintf("NODE_ENV=production"),
		fmt.Sprintf("NPM_CONFIG_CACHE=%s", filepath.Join(envDir, ".npm-cache")),
		fmt.Sprintf("NPM_CONFIG_TMP=%s", filepath.Join(envDir, ".npm-tmp")),
		"NPM_CONFIG_IGNORE_SCRIPTS=true",
		"NPM_CONFIG_FUND=false",
		"NPM_CONFIG_AUDIT=false",
	}

	// Generate lock file first
	lockArgs := []string{
		"install",
		"--package-lock-only",
		"--ignore-scripts",
		"--no-audit",
		"--fund=false",
	}
	if err := m.runInstallCmd(ctx, m.cfg.NPMCommand, lockArgs, envDir, env); err != nil {
		return err
	}
	lockHash, err := fileChecksum(filepath.Join(envDir, "package-lock.json"))
	if err != nil {
		return fmt.Errorf("package-lock.json missing after generation: %w", err)
	}

	// Install strictly from lock
	installArgs := []string{
		"ci",
		"--ignore-scripts",
		"--no-audit",
		"--fund=false",
		"--production",
	}
	if err := m.runInstallCmd(ctx, m.cfg.NPMCommand, installArgs, envDir, env); err != nil {
		return err
	}
	lockHashAfter, err := fileChecksum(filepath.Join(envDir, "package-lock.json"))
	if err != nil {
		return fmt.Errorf("package-lock.json missing after install: %w", err)
	}
	if lockHash != "" && lockHashAfter != lockHash {
		return fmt.Errorf("package-lock.json was modified during install (expected %s got %s)", lockHash, lockHashAfter)
	}
	return nil
}

func (m *Manager) writePackageJSON(dir string, packages []string) error {
	type pkgJSON struct {
		Name         string            `json:"name"`
		Version      string            `json:"version"`
		Private      bool              `json:"private"`
		Dependencies map[string]string `json:"dependencies"`
	}

	deps := make(map[string]string)
	for _, p := range packages {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		name, version := splitNpmPackage(p)
		deps[name] = version
	}

	data := pkgJSON{
		Name:         "plugin-node-runtime",
		Version:      "1.0.0",
		Private:      true,
		Dependencies: deps,
	}

	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "package.json"), b, 0o644)
}

func splitNpmPackage(p string) (string, string) {
	if strings.HasPrefix(p, "@") {
		parts := strings.SplitN(p[1:], "@", 2)
		if len(parts) == 2 {
			return "@" + parts[0], parts[1]
		}
		return "@" + parts[0], "latest"
	}
	parts := strings.SplitN(p, "@", 2)
	if len(parts) == 2 && parts[0] != "" {
		return parts[0], parts[1]
	}
	return p, "latest"
}

func fileChecksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (m *Manager) precompile(ctx context.Context, pythonPath, workdir string) error {
	args := []string{"-m", "compileall"}
	if strings.TrimSpace(m.cfg.PythonCompileExtra) != "" {
		args = append(args, strings.Fields(m.cfg.PythonCompileExtra)...)
	}
	args = append(args, ".")

	execCmd := exec.CommandContext(ctx, pythonPath, args...)
	execCmd.Dir = workdir
	if output, err := execCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("compileall failed: %w, output: %s", err, string(output))
	}
	return nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (m *Manager) runInstallCmd(ctx context.Context, cmd string, args []string, workdir string, env []string) error {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return fmt.Errorf("install command is empty")
	}

	ctx, cancel := context.WithTimeout(ctx, m.cfg.InstallTimeout)
	defer cancel()

	execCmd := exec.CommandContext(ctx, cmd, args...)
	execCmd.Dir = workdir
	execCmd.Env = append(execCmd.Env, env...)
	execCmd.Env = append(execCmd.Env, proxyEnv(m.cfg)...)

	output, err := execCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("install dependencies failed: %w, output: %s", err, string(output))
	}
	m.log.Info("dependencies installed",
		zap.String("cmd", cmd),
		zap.Strings("args", args),
		zap.String("workdir", workdir),
	)
	return nil
}

func proxyEnv(cfg *config.Config) []string {
	var env []string
	if cfg.HTTPProxy != "" {
		env = append(env, fmt.Sprintf("HTTP_PROXY=%s", cfg.HTTPProxy))
	}
	if cfg.HTTPSProxy != "" {
		env = append(env, fmt.Sprintf("HTTPS_PROXY=%s", cfg.HTTPSProxy))
	}
	if cfg.NoProxy != "" {
		env = append(env, fmt.Sprintf("NO_PROXY=%s", cfg.NoProxy))
	}
	return env
}
