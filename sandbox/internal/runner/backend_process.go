package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type processBackend struct {
	dependencyRootFSDir string
}

func newProcessBackend(dependencyRootFSDir ...string) backend {
	root := ""
	if len(dependencyRootFSDir) > 0 {
		root = strings.TrimSpace(dependencyRootFSDir[0])
	}
	return &processBackend{dependencyRootFSDir: root}
}

func (b *processBackend) Name() string {
	return "preview-process"
}

func (b *processBackend) Run(parent context.Context, req Request, workDir string, ephemeral bool, timeout time.Duration, stdoutLimit int, stderrLimit int) (Result, error) {
	spec, err := languageSpec(req.Language)
	if err != nil {
		return Result{}, err
	}

	runCtx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	root := workDir
	if root == "" {
		root, err = os.MkdirTemp("", "zgi-sandbox-*")
		if err != nil {
			return Result{}, err
		}
	}
	if ephemeral {
		defer os.RemoveAll(root)
	}

	scriptPath := filepath.Join(root, fmt.Sprintf(".zgi-run-%s-%s", token(), spec.filename))
	content := buildContent(req.Preload, req.Code)
	if err := os.WriteFile(scriptPath, []byte(content), 0o600); err != nil {
		return Result{}, err
	}
	defer os.Remove(scriptPath)

	activation, err := b.resolveDependencyProfile(req.DependencyProfile, req.DependencyArtifactChecksum)
	if err != nil {
		return Result{}, err
	}
	binary := processProfileCommandPath(activation.ProfileHostDir, spec.binary)
	cmd := exec.CommandContext(runCtx, binary, spec.args(scriptPath)...)
	cmd.Dir = root
	if req.Stdin != "" {
		cmd.Stdin = strings.NewReader(req.Stdin)
	}
	cmd.Env = processEnv(nil, activation.ProfileEnv)

	stdout := newCappedBuffer(stdoutLimit)
	stderr := newCappedBuffer(stderrLimit)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	started := time.Now()
	err = runProcessGroup(runCtx, cmd)
	duration := time.Since(started).Milliseconds()

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		switch {
		case errors.Is(runCtx.Err(), context.DeadlineExceeded):
			exitCode = 124
			stderr.AppendLine("execution timed out")
		case errors.As(err, &exitErr):
			exitCode = exitCodeFromExitError(exitErr, stderr)
		default:
			return Result{}, err
		}
	}

	return Result{
		Stdout:           stdout.String(),
		Error:            stderr.String(),
		ExitCode:         exitCode,
		DurationMS:       duration,
		NetworkRequested: req.EnableNetwork,
		Truncated:        stdout.Truncated() || stderr.Truncated(),
		ProfileChecksum:  activation.ProfileChecksum,
	}, nil
}

func (b *processBackend) ExecuteCommand(parent context.Context, spec CommandSpec) (CommandResult, error) {
	runCtx, cancel := context.WithTimeout(parent, spec.Timeout)
	defer cancel()

	activation, err := b.resolveDependencyProfile(spec.DependencyProfile, spec.DependencyArtifactChecksum)
	if err != nil {
		return CommandResult{}, err
	}
	command := processProfileCommandPath(activation.ProfileHostDir, spec.Command)
	var cmd *exec.Cmd
	if len(spec.Args) > 0 {
		cmd = exec.CommandContext(runCtx, command, spec.Args...)
	} else if spec.AllowShellForm {
		cmd = exec.CommandContext(runCtx, "/bin/sh", "-lc", spec.Command)
	} else {
		cmd = exec.CommandContext(runCtx, command)
	}
	cmd.Dir = spec.WorkDir
	if spec.Stdin != "" {
		cmd.Stdin = strings.NewReader(spec.Stdin)
	}
	cmd.Env = processEnv(spec.Env, activation.ProfileEnv)

	stdout := newCappedBuffer(spec.StdoutLimit)
	stderr := newCappedBuffer(spec.StderrLimit)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	started := time.Now()
	err = runProcessGroup(runCtx, cmd)
	duration := time.Since(started).Milliseconds()

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		switch {
		case errors.Is(runCtx.Err(), context.DeadlineExceeded):
			exitCode = 124
			stderr.AppendLine("command timed out")
		case errors.As(err, &exitErr):
			exitCode = exitCodeFromExitError(exitErr, stderr)
		default:
			return CommandResult{}, err
		}
	}

	return CommandResult{
		Stdout:          stdout.String(),
		Error:           stderr.String(),
		ExitCode:        exitCode,
		DurationMS:      duration,
		Truncated:       stdout.Truncated() || stderr.Truncated(),
		Command:         spec.Command,
		Args:            spec.Args,
		ProfileChecksum: activation.ProfileChecksum,
	}, nil
}

func (b *processBackend) resolveDependencyProfile(dependencyProfile string, dependencyArtifactChecksum string) (dependencyProfileActivation, error) {
	dependencyProfile = strings.TrimSpace(dependencyProfile)
	if dependencyProfile == "" || strings.TrimSpace(b.dependencyRootFSDir) == "" {
		return dependencyProfileActivation{}, nil
	}
	if !safeDependencyProfileName(dependencyProfile) {
		return dependencyProfileActivation{}, ErrUnsafeDependencyProfileName{Profile: dependencyProfile}
	}
	root, err := rootFSSelector{
		defaultRootFS:       b.dependencyRootFSDir,
		dependencyRootFSDir: b.dependencyRootFSDir,
	}.resolve(dependencyProfile, dependencyArtifactChecksum)
	if err != nil {
		return dependencyProfileActivation{}, err
	}
	profileDir, manifest, err := findRuntimeProfileArtifact(root, dependencyProfile, dependencyArtifactChecksum)
	if err != nil {
		return dependencyProfileActivation{}, err
	}
	return dependencyProfileActivation{
		RootFS:           root,
		ProfileName:      dependencyProfile,
		ProfileHostDir:   profileDir,
		ProfileChecksum:  manifest.Build.Checksum,
		ProfileSizeBytes: manifest.Build.SizeBytes,
		ProfileEnv:       processDependencyProfileEnv(profileDir),
	}, nil
}

func processDependencyProfileEnv(profileDir string) map[string]string {
	profileDir = strings.TrimSpace(profileDir)
	if profileDir == "" {
		return nil
	}
	path := filepath.Join(profileDir, "venv", "bin") + string(os.PathListSeparator) + filepath.Join(profileDir, "node_modules", ".bin")
	if existing := strings.TrimSpace(os.Getenv("PATH")); existing != "" {
		path += string(os.PathListSeparator) + existing
	}
	return map[string]string{
		"PATH":             path,
		"PYTHONNOUSERSITE": "1",
		"NODE_PATH":        filepath.Join(profileDir, "node_modules"),
	}
}

func processProfileCommandPath(profileDir string, command string) string {
	profileDir = strings.TrimSpace(profileDir)
	command = strings.TrimSpace(command)
	if profileDir == "" || command == "" || strings.ContainsAny(command, `/\`) {
		return command
	}
	candidates := []string{}
	switch command {
	case "python", "python3":
		candidates = append(candidates,
			filepath.Join(profileDir, "venv", "bin", command),
			filepath.Join(profileDir, "venv", "bin", "python"),
			filepath.Join(profileDir, "venv", "Scripts", command+".exe"),
			filepath.Join(profileDir, "venv", "Scripts", "python.exe"),
		)
	case "node":
		candidates = append(candidates,
			filepath.Join(profileDir, "node", "bin", "node"),
			filepath.Join(profileDir, "node_modules", ".bin", "node"),
		)
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return command
}

func processEnv(requestEnv map[string]string, profileEnv map[string]string) []string {
	env := safeBaseEnv(os.Environ())
	for key, value := range requestEnv {
		env = setProcessEnv(env, key, value)
	}
	for key, value := range profileEnv {
		env = setProcessEnv(env, key, value)
	}
	return env
}

func setProcessEnv(env []string, key string, value string) []string {
	key = strings.TrimSpace(key)
	if key == "" {
		return env
	}
	prefix := key + "="
	item := prefix + value
	for index, existing := range env {
		if strings.HasPrefix(existing, prefix) {
			env[index] = item
			return env
		}
	}
	return append(env, item)
}

func runProcessGroup(ctx context.Context, cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		killProcessGroup(cmd)
		<-done
		return ctx.Err()
	}
}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	_ = cmd.Process.Kill()
}
