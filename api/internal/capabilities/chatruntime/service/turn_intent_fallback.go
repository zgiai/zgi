package service

func partsModelTurnIntentName(parts *chatRequestParts) string {
	if parts == nil || parts.ModelTurnIntent == nil {
		return ""
	}
	return normalizeModelTurnIntent(parts.ModelTurnIntent.Intent)
}

func partsRequestsManagedFileCreate(parts *chatRequestParts) bool {
	return partsRequestsManagedFileCreateWithFallback(parts, "")
}

func partsRequestsManagedFileCreateWithFallback(parts *chatRequestParts, fallback string) bool {
	return partsModelTurnIntentName(parts) == "save_generated_file_to_file_management"
}

func partsRequestsTemporaryFileGenerateWithFallback(parts *chatRequestParts, fallback string) bool {
	if parts == nil {
		return false
	}
	return modelTurnIntentRequestsTemporaryFileArtifact(parts.ModelTurnIntent)
}

func partsRequestsFileDeleteWithFallback(parts *chatRequestParts, fallback string) bool {
	return partsModelTurnIntentName(parts) == "delete_visible_file"
}

func partsRequestsFileReadWithFallback(parts *chatRequestParts, fallback string) bool {
	intent := partsModelTurnIntentName(parts)
	if intent == "read_visible_file_content" {
		return true
	}
	if parts != nil && modelTurnIntentHasRecommendedCapability(parts.ModelTurnIntent, "visible_file_content", "file_content", "source_file_content") {
		return true
	}
	return false
}

func partsRequestsContinuationWithFallback(parts *chatRequestParts, fallback string) bool {
	if partsModelTurnIntentName(parts) == "continue_previous_task" {
		return true
	}
	text := fallback
	if text == "" && parts != nil {
		text = parts.Query
	}
	if partsModelTurnIntentName(parts) != "" {
		return isExplicitContinuationCommand(text)
	}
	return isContinuationIntent(text)
}

func partsRequestsConsoleNavigationWithFallback(parts *chatRequestParts, fallback string) bool {
	return partsModelTurnIntentName(parts) == "navigate_console_page"
}

func consoleNavigationResolvedTargetsForParts(parts *chatRequestParts) []consoleNavigationRouteHint {
	if parts == nil {
		return nil
	}
	if partsModelTurnIntentName(parts) != "navigate_console_page" {
		return nil
	}
	if target := consoleNavigationRouteHintFromModelIntent(parts.ModelTurnIntent); target.Href != "" {
		return []consoleNavigationRouteHint{target}
	}
	return nil
}

func consoleNavigationRouteHintFromModelIntent(intent *AIChatModelTurnIntent) consoleNavigationRouteHint {
	if intent == nil {
		return consoleNavigationRouteHint{}
	}
	targetPage := normalizeConsoleNavigationGuardHref(intent.TargetPage)
	if targetPage == "" {
		return consoleNavigationRouteHint{}
	}
	return consoleNavigationRouteHintForHref(targetPage)
}
