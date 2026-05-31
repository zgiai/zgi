package runner

import (
	"strconv"

	"github.com/zgiai/zgi-sandbox/internal/config"
)

type secureRuntimeLimits struct {
	CPUSeconds    int
	MemoryBytes   int64
	ProcessLimit  int
	OpenFileLimit int
}

func secureRuntimeLimitsFromConfig(cfg config.Config) secureRuntimeLimits {
	return secureRuntimeLimits{
		CPUSeconds:    cfg.SecureRuntimeCPUSeconds,
		MemoryBytes:   cfg.SecureRuntimeMemoryBytes,
		ProcessLimit:  cfg.SecureRuntimeProcessLimit,
		OpenFileLimit: cfg.SecureRuntimeOpenFileLimit,
	}
}

func (l secureRuntimeLimits) bwrapArgs() []string {
	args := []string{}
	if l.CPUSeconds > 0 {
		args = append(args, "--rlimit", "cpu", strconv.Itoa(l.CPUSeconds))
	}
	if l.MemoryBytes > 0 {
		args = append(args, "--rlimit", "as", strconv.FormatInt(l.MemoryBytes, 10))
	}
	if l.ProcessLimit > 0 {
		args = append(args, "--rlimit", "nproc", strconv.Itoa(l.ProcessLimit))
	}
	if l.OpenFileLimit > 0 {
		args = append(args, "--rlimit", "nofile", strconv.Itoa(l.OpenFileLimit))
	}
	return args
}
