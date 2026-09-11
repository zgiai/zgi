package tools

import "github.com/zgiai/zgi/api/internal/capabilities/connectortools"

// Persist only host-authored outcome facts. Provider payloads and confirmation
// commands never enter the read-only node outcome projection.
func workflowConnectorOutcomePresentation(metadata connectortools.ResultMetadata) map[string]interface{} {
	result := map[string]interface{}{
		"state": metadata.State, "operation": metadata.Operation,
		"outcome_id": metadata.OutcomeID, "outcome_status": metadata.OutcomeStatus,
		"connector_presentation": metadata.Presentation,
	}
	if metadata.ExecutionEvidence != nil {
		result["execution_evidence"] = metadata.ExecutionEvidence
	}
	return result
}
