package workflow

import "github.com/google/uuid"

func workflowVersionSelectorID(workflow *Workflow) string {
	if workflow == nil {
		return ""
	}
	if workflow.VersionUUID.Valid && workflow.VersionUUID.UUID != uuid.Nil {
		return workflow.VersionUUID.UUID.String()
	}
	return workflow.ID
}
