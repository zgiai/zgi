package workflow

import workflow_shared "github.com/zgiai/ginext/internal/modules/app/workflow/shared"

func workflowExecutionMetadataToMap(metadata map[workflow_shared.WorkflowNodeExecutionMetadataKey]any) map[string]interface{} {
	if len(metadata) == 0 {
		return nil
	}

	result := make(map[string]interface{}, len(metadata))
	for key, value := range metadata {
		result[string(key)] = value
	}

	return result
}
