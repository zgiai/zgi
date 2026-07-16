package runner

import (
	"sort"
	"strings"
)

const secureWorkspacePath = "/tmp/workspace"
const defaultSecurePath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

type secureBwrapSpec struct {
	RootFS              string
	WorkDir             string
	Binary              string
	Args                []string
	EnableNetwork       bool
	Env                 map[string]string
	ProfileEnv          map[string]string
	ProfileHostDir      string
	ProfileContainerDir string
}

func buildSecureBwrapArgs(spec secureBwrapSpec) []string {
	// Keep procfs empty so nested container runtimes cannot expose parent process metadata.
	args := []string{
		"--die-with-parent",
		"--new-session",
		"--clearenv",
		"--ro-bind", spec.RootFS, "/",
		"--dir", "/proc",
		"--dev", "/dev",
		"--tmpfs", "/tmp",
		"--dir", secureWorkspacePath,
		"--bind", spec.WorkDir, secureWorkspacePath,
		"--chdir", secureWorkspacePath,
		"--setenv", "HOME", secureWorkspacePath,
		"--setenv", "PATH", defaultSecurePath,
		"--unshare-user",
		"--uid", "65534",
		"--gid", "65534",
		"--unshare-pid",
		"--unshare-ipc",
		"--unshare-uts",
		"--unshare-cgroup",
	}
	for _, key := range sortedEnvKeys(spec.Env) {
		args = append(args, "--setenv", key, spec.Env[key])
	}
	for _, key := range sortedEnvKeys(spec.ProfileEnv) {
		args = append(args, "--setenv", key, spec.ProfileEnv[key])
	}
	if spec.ProfileHostDir != "" && spec.ProfileContainerDir != "" {
		args = append(args, "--ro-bind", spec.ProfileHostDir, spec.ProfileContainerDir)
	}
	if !spec.EnableNetwork {
		args = append(args, "--unshare-net")
	}
	args = append(args, spec.Binary)
	args = append(args, spec.Args...)
	return args
}

func sortedEnvKeys(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func secureDependencyProfileEnv(profile string) (map[string]string, error) {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return nil, nil
	}
	if !safeDependencyProfileName(profile) {
		return nil, ErrUnsafeDependencyProfileName{Profile: profile}
	}
	base := "/opt/zgi/profiles/" + profile
	return map[string]string{
		"PATH":             base + "/venv/bin:" + base + "/node_modules/.bin:" + defaultSecurePath,
		"PYTHONNOUSERSITE": "1",
		"NODE_PATH":        base + "/node_modules",
	}, nil
}
