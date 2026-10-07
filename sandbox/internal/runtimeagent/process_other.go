//go:build !linux

package runtimeagent

import (
	"context"
	"fmt"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func supportsPTY() bool { return false }

func startProcess(context.Context, string, agentexecution.JobSpec, uint32, uint32, func(agentexecution.OutputStream, []byte) error, func(error)) (agentexecution.RunningJob, error) {
	return nil, fmt.Errorf("%w: the in-environment runtime agent requires Linux", agentexecution.ErrBackendUnavailable)
}
