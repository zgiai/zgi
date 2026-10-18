package runner

const secureCgroupLauncherScript = `printf '%s\n' "$$" > "$1/cgroup.procs" || exit 125
shift
exec "$@"`

func wrapSecureCommandInCgroup(shellBin, cgroupPath, command string, commandArgs []string, bwrapArgIndex int) (string, []string, int) {
	const wrapperArgCount = 5
	args := make([]string, 0, 4+1+len(commandArgs))
	args = append(args, "-c", secureCgroupLauncherScript, "zgi-cgroup-launch", cgroupPath, command)
	args = append(args, commandArgs...)
	return shellBin, args, bwrapArgIndex + wrapperArgCount
}
