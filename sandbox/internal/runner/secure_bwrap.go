package runner

import (
	"sort"
)

const secureWorkspacePath = "/tmp/workspace"

type secureBwrapSpec struct {
	RootFS        string
	WorkDir       string
	Binary        string
	Args          []string
	EnableNetwork bool
	Env           map[string]string
	Limits        secureRuntimeLimits
}

func buildSecureBwrapArgs(spec secureBwrapSpec) []string {
	args := []string{
		"--die-with-parent",
		"--new-session",
		"--clearenv",
		"--ro-bind", spec.RootFS, "/",
		"--proc", "/proc",
		"--dev", "/dev",
		"--tmpfs", "/tmp",
		"--dir", secureWorkspacePath,
		"--bind", spec.WorkDir, secureWorkspacePath,
		"--chdir", secureWorkspacePath,
		"--setenv", "HOME", secureWorkspacePath,
		"--setenv", "PATH", "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"--unshare-user",
		"--uid", "65534",
		"--gid", "65534",
		"--unshare-pid",
		"--unshare-ipc",
		"--unshare-uts",
		"--unshare-cgroup",
	}
	args = append(args, spec.Limits.bwrapArgs()...)
	for _, key := range sortedEnvKeys(spec.Env) {
		args = append(args, "--setenv", key, spec.Env[key])
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
