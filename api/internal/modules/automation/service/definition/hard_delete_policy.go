package definition

import (
	"errors"
	"fmt"

	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
)

var (
	// ErrTaskMustBeArchivedForHardDelete ensures physical deletion only happens after archive.
	ErrTaskMustBeArchivedForHardDelete = errors.New("automation task must be archived before hard delete")
	// ErrTaskHasRuns keeps audit semantics conservative until a cleanup strategy is finalized.
	ErrTaskHasRuns = errors.New("automation task with execution history cannot be hard deleted")
)

// ValidateHardDeleteTask validates whether a task can be physically removed in MVP.
func ValidateHardDeleteTask(task *automationmodel.AutomationTask, runCount int64) error {
	if task == nil {
		return fmt.Errorf("automation task is nil")
	}
	if task.Status != automationmodel.AutomationTaskStatusArchived {
		return ErrTaskMustBeArchivedForHardDelete
	}
	if runCount > 0 {
		return ErrTaskHasRuns
	}
	if err := validateReservedHardDeleteConstraints(task); err != nil {
		return err
	}
	return nil
}

// validateReservedHardDeleteConstraints is the extension point for future business reference checks.
func validateReservedHardDeleteConstraints(task *automationmodel.AutomationTask) error {
	_ = task
	return nil
}
