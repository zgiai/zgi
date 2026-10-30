package service

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

func turnTaskContractRequestsManagedFileCreate(parts *chatRequestParts, metadata map[string]interface{}, fallback string) bool {
	switch turnTaskContractIntentLabel(turnTaskContractFromPartsAndMetadata(parts, metadata)) {
	case "save_generated_file_to_file_management":
		return true
	default:
		return false
	}
}
