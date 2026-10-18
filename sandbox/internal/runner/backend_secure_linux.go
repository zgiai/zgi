//go:build linux

package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/config"
)

type linuxSecureBackend struct {
	rootfs              string
	dependencyRootFSDir string
	bwrapBin            string
	prlimitBin          string
	shellBin            string
	limits              secureRuntimeLimits
	processCgroups      *processCgroupManager
	allowShell          bool
	profileCache        *dependencyProfileActivationCache
}

func newLinuxSecureBackend(cfg config.Config) (backend, error) {
	rootfs := strings.TrimSpace(cfg.SecureRootFS)
	if rootfs == "" {
		return nil, errors.New("linux-secure backend requires ZGI_SANDBOX_SECURE_ROOTFS")
	}
	if _, err := os.Stat(rootfs); err != nil {
		return nil, fmt.Errorf("stat secure rootfs: %w", err)
	}

	bwrapBin := strings.TrimSpace(cfg.BwrapBinary)
	if bwrapBin == "" {
		bwrapBin = "bwrap"
	}
	if _, err := exec.LookPath(bwrapBin); err != nil {
		return nil, fmt.Errorf("find bubblewrap binary: %w", err)
	}

	limits := secureRuntimeLimitsFromConfig(cfg)
	prlimitBin := ""
	if len(limits.prlimitArgs()) > 0 {
		resolvedPrlimitBin, err := exec.LookPath("prlimit")
		if err != nil {
			return nil, fmt.Errorf("find prlimit binary: %w", err)
		}
		prlimitBin = resolvedPrlimitBin
	}
	shellBin := ""
	var processCgroups *processCgroupManager
	if limits.ProcessLimit > 0 {
		resolvedShellBin, err := exec.LookPath("sh")
		if err != nil {
			return nil, fmt.Errorf("find shell for secure runtime cgroup launcher: %w", err)
		}
		shellBin = resolvedShellBin
		processCgroups, err = newProcessCgroupManager(cfg.SecureRuntimeCgroupRootPath(), limits.ProcessLimit)
		if err != nil {
			return nil, fmt.Errorf("configure secure runtime process cgroups: %w", err)
		}
	}

	return &linuxSecureBackend{
		rootfs:              rootfs,
		dependencyRootFSDir: strings.TrimSpace(cfg.DependencyRootFSDir),
		bwrapBin:            bwrapBin,
		prlimitBin:          prlimitBin,
		shellBin:            shellBin,
		limits:              limits,
		processCgroups:      processCgroups,
		allowShell:          true,
		profileCache:        newDependencyProfileActivationCache(),
	}, nil
}

func (b *linuxSecureBackend) Name() string {
	return "linux-secure"
}

func (b *linuxSecureBackend) Run(parent context.Context, req Request, workDir string, ephemeral bool, timeout time.Duration, stdoutLimit int, stderrLimit int) (Result, error) {
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

	scriptName := fmt.Sprintf(".zgi-run-%s-%s", token(), spec.filename)
	hostScriptPath := filepath.Join(root, scriptName)
	if err := os.WriteFile(hostScriptPath, []byte(buildContent(req.Preload, req.Code)), 0o600); err != nil {
		return Result{}, err
	}
	defer os.Remove(hostScriptPath)

	containerPath := containerScriptPath(root, scriptName)
	return b.exec(runCtx, root, req.DependencyProfile, req.DependencyArtifactChecksum, spec.binary, spec.args(containerPath), req.EnableNetwork, stdoutLimit, stderrLimit, req.Stdin, nil)
}

func (b *linuxSecureBackend) ExecuteCommand(parent context.Context, spec CommandSpec) (CommandResult, error) {
	runCtx, cancel := context.WithTimeout(parent, spec.Timeout)
	defer cancel()

	commandArgs := []string{}
	if len(spec.Args) > 0 {
		commandArgs = append(commandArgs, spec.Command)
		commandArgs = append(commandArgs, spec.Args...)
	} else if b.allowShell && spec.AllowShellForm {
		commandArgs = []string{"/bin/sh", "-lc", spec.Command}
	} else {
		commandArgs = []string{spec.Command}
	}

	result, err := b.exec(runCtx, spec.WorkDir, spec.DependencyProfile, spec.DependencyArtifactChecksum, commandArgs[0], commandArgs[1:], false, spec.StdoutLimit, spec.StderrLimit, spec.Stdin, spec.Env)
	if err != nil {
		return CommandResult{}, err
	}
	return CommandResult{
		Stdout:          result.Stdout,
		Error:           result.Error,
		ExitCode:        result.ExitCode,
		DurationMS:      result.DurationMS,
		Truncated:       result.Truncated,
		Command:         spec.Command,
		Args:            spec.Args,
		ProfileChecksum: result.ProfileChecksum,
	}, nil
}

func (b *linuxSecureBackend) exec(ctx context.Context, workDir string, dependencyProfile string, dependencyArtifactChecksum string, binary string, args []string, enableNetwork bool, stdoutLimit int, stderrLimit int, stdin string, env map[string]string) (_ Result, returnErr error) {
	activation, err := b.profileCache.resolve(b.rootfs, b.dependencyRootFSDir, dependencyProfile, dependencyArtifactChecksum)
	if err != nil {
		return Result{}, err
	}
	if err := seedDependencyProfileRuntimeCache(activation.ProfileHostDir, workDir); err != nil {
		return Result{}, err
	}

	bwrapArgs := buildSecureBwrapArgs(secureBwrapSpec{
		RootFS:              activation.RootFS,
		WorkDir:             workDir,
		Binary:              binary,
		Args:                args,
		EnableNetwork:       enableNetwork,
		Env:                 env,
		ProfileEnv:          activation.ProfileEnv,
		ProfileHostDir:      activation.ProfileHostDir,
		ProfileContainerDir: activation.ProfileContainerDir,
	})

	command := b.bwrapBin
	commandArgs := bwrapArgs
	bwrapArgIndex := 0
	if limitArgs := b.limits.prlimitArgs(); len(limitArgs) > 0 {
		command = b.prlimitBin
		commandArgs = make([]string, 0, len(limitArgs)+2+len(bwrapArgs))
		commandArgs = append(commandArgs, limitArgs...)
		commandArgs = append(commandArgs, "--", b.bwrapBin)
		commandArgs = append(commandArgs, bwrapArgs...)
		bwrapArgIndex = len(limitArgs) + 2
	}
	if b.processCgroups != nil {
		executionCgroup, err := b.processCgroups.Create()
		if err != nil {
			return Result{}, err
		}
		defer func() {
			returnErr = errors.Join(returnErr, executionCgroup.Close())
		}()
		command, commandArgs, bwrapArgIndex = wrapSecureCommandInCgroup(
			b.shellBin,
			executionCgroup.path,
			command,
			commandArgs,
			bwrapArgIndex,
		)
	}

	cmd := exec.CommandContext(ctx, command, commandArgs...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	stdout := newCappedBuffer(stdoutLimit)
	stderr := newCappedBuffer(stderrLimit)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	started := time.Now()
	err = runSecureProcess(ctx, cmd, bwrapArgIndex)
	duration := time.Since(started).Milliseconds()

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			exitCode = 124
			stderr.AppendLine("execution timed out")
		case errors.As(err, &exitErr):
			exitCode = exitCodeFromExitError(exitErr, stderr)
		default:
			return Result{}, err
		}
	}

	return Result{
		Stdout:          stdout.String(),
		Error:           stderr.String(),
		ExitCode:        exitCode,
		DurationMS:      duration,
		Truncated:       stdout.Truncated() || stderr.Truncated(),
		ProfileChecksum: activation.ProfileChecksum,
	}, nil
}
