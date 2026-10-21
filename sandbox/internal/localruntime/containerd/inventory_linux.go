//go:build linux

package containerd

import (
	"context"
	"fmt"

	containerdclient "github.com/containerd/containerd/v2/client"
	"github.com/containerd/errdefs"
	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

// ReadInventory is deliberately separate from New: no directories, mounts,
// tokens, processes or runtime connections are created by an operator audit.
func ReadInventory(ctx context.Context, address string) ([]agentexecution.ResourceObservation, error) {
	client, err := containerdclient.New(address, containerdclient.WithDefaultNamespace(defaultNamespace))
	if err != nil {
		return nil, err
	}
	defer client.Close()
	containers, err := client.Containers(ctx, `labels."zgi.agent-runtime.managed"==true`)
	if err != nil {
		return nil, err
	}
	if len(containers) > 10000 {
		return nil, fmt.Errorf("container inventory exceeds 10000 resources")
	}
	result := make([]agentexecution.ResourceObservation, 0, len(containers))
	for _, container := range containers {
		labels, err := container.Labels(ctx)
		if err != nil {
			return nil, err
		}
		state := "stopped"
		task, err := container.Task(ctx, nil)
		if err != nil && !errdefs.IsNotFound(err) {
			return nil, err
		}
		if err == nil {
			status, err := task.Status(ctx)
			if err != nil {
				return nil, err
			}
			state = string(status.Status)
		}
		result = append(result, agentexecution.ResourceObservation{Ref: container.ID(), BindingID: labels["zgi.agent-runtime.binding"], State: state})
	}
	return result, nil
}
