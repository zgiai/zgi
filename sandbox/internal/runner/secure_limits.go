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

func (l secureRuntimeLimits) prlimitArgs() []string {
	args := []string{}
	if l.CPUSeconds > 0 {
		args = append(args, "--cpu="+strconv.Itoa(l.CPUSeconds))
	}
	if l.MemoryBytes > 0 {
		args = append(args, "--as="+strconv.FormatInt(l.MemoryBytes, 10))
	}
	if l.ProcessLimit > 0 {
		args = append(args, "--nproc="+strconv.Itoa(l.ProcessLimit))
	}
	if l.OpenFileLimit > 0 {
		args = append(args, "--nofile="+strconv.Itoa(l.OpenFileLimit))
	}
	return args
}
