package workflow

import "fmt"

func wrapNodeExecutionError(nodeID string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("node %s failed: %w", nodeID, err)
}
