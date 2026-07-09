package service

import "strings"

func turnTaskContractFromPartsAndMetadata(parts *chatRequestParts, metadata map[string]interface{}) map[string]interface{} {
	if contract := mapFromOperationContext(metadataValue(metadata, "turn_task_contract")); len(contract) > 0 {
		return contract
	}
	if plan := mapFromOperationContext(metadataValue(metadata, "operation_plan")); len(plan) > 0 {
		if contract := mapFromOperationContext(plan["task_contract"]); len(contract) > 0 {
			return contract
		}
	}
	if parts != nil && parts.ModelTurnIntent != nil {
		return modelTurnIntentTaskContract(parts.ModelTurnIntent)
	}
	return nil
}

func turnTaskContractIntentLabel(contract map[string]interface{}) string {
	if len(contract) == 0 {
		return ""
	}
	return normalizeModelTurnIntent(firstNonEmptyString(contract["intent_label"], contract["intent"]))
}

func turnTaskContractHasRecommendedCapability(contract map[string]interface{}, values ...string) bool {
	if len(contract) == 0 || len(values) == 0 {
		return false
	}
	wanted := map[string]struct{}{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			wanted[value] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return false
	}
	for _, candidate := range stringSliceFromAny(contract["recommended_capabilities"]) {
		normalized := strings.ToLower(strings.TrimSpace(candidate))
		if _, ok := wanted[normalized]; ok {
			return true
		}
	}
	return false
}

func turnTaskContractRequestsManagedFileCreate(parts *chatRequestParts, metadata map[string]interface{}, fallback string) bool {
	switch turnTaskContractIntentLabel(turnTaskContractFromPartsAndMetadata(parts, metadata)) {
	case "save_generated_file_to_file_management":
		return true
	case "continue_previous_task":
		return partsRequestsManagedFileCreateWithFallback(parts, fallback)
	case "":
		return partsAllowsLegacyBusinessIntentFallback(parts) &&
			partsRequestsManagedFileCreateWithFallback(parts, fallback)
	default:
		return false
	}
}

func turnTaskContractRequestsTemporaryFileGenerate(parts *chatRequestParts, metadata map[string]interface{}, fallback string) bool {
	contract := turnTaskContractFromPartsAndMetadata(parts, metadata)
	switch turnTaskContractIntentLabel(contract) {
	case "generate_temporary_file_artifact":
		return true
	case "continue_previous_task":
		return partsRequestsTemporaryFileGenerateWithFallback(parts, fallback)
	case "":
		return partsAllowsLegacyBusinessIntentFallback(parts) &&
			partsRequestsTemporaryFileGenerateWithFallback(parts, fallback)
	default:
		return turnTaskContractHasRecommendedCapability(contract,
			"chart_artifact",
			"data_visualization_artifact",
			"visualization_artifact",
			"file_artifact",
			"document_artifact",
			"svg_artifact",
			"text_artifact",
			"pdf_artifact",
			"spreadsheet_artifact",
		)
	}
}

func turnTaskContractRequestsFileDelete(parts *chatRequestParts, metadata map[string]interface{}, fallback string) bool {
	switch turnTaskContractIntentLabel(turnTaskContractFromPartsAndMetadata(parts, metadata)) {
	case "delete_visible_file":
		return true
	case "continue_previous_task":
		return partsRequestsFileDeleteWithFallback(parts, fallback)
	case "":
		return partsAllowsLegacyBusinessIntentFallback(parts) &&
			partsRequestsFileDeleteWithFallback(parts, fallback)
	default:
		return false
	}
}

func turnTaskContractRequestsFileRead(parts *chatRequestParts, metadata map[string]interface{}, fallback string) bool {
	contract := turnTaskContractFromPartsAndMetadata(parts, metadata)
	switch turnTaskContractIntentLabel(contract) {
	case "read_visible_file_content":
		return true
	case "continue_previous_task":
		return partsRequestsFileReadWithFallback(parts, fallback)
	case "":
		return partsAllowsLegacyBusinessIntentFallback(parts) &&
			partsRequestsFileReadWithFallback(parts, fallback)
	default:
		return turnTaskContractHasRecommendedCapability(contract, "visible_file_content", "file_content", "source_file_content")
	}
}

func turnTaskContractRequestsConsoleNavigation(parts *chatRequestParts, metadata map[string]interface{}, fallback string) bool {
	switch turnTaskContractIntentLabel(turnTaskContractFromPartsAndMetadata(parts, metadata)) {
	case "navigate_console_page":
		return true
	case "continue_previous_task":
		return partsRequestsConsoleNavigationWithFallback(parts, fallback)
	case "":
		return partsAllowsLegacyBusinessIntentFallback(parts) &&
			partsRequestsConsoleNavigationWithFallback(parts, fallback)
	default:
		return false
	}
}
