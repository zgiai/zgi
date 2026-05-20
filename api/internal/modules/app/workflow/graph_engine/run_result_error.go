package graph_engine

import (
	"errors"
	"fmt"

	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

func errorFromNodeRunResult(result *shared.NodeRunResult) error {
	if result == nil {
		return nil
	}
	if result.Err != nil {
		return result.Err
	}
	if result.ErrMsg != "" {
		return errors.New(result.ErrMsg)
	}
	return fmt.Errorf("node execution failed with status: %s", result.Status)
}
