package service

import "strings"

func partsModelTurnIntentName(parts *chatRequestParts) string {
	if parts == nil || parts.ModelTurnIntent == nil {
		return ""
	}
	return normalizeModelTurnIntent(parts.ModelTurnIntent.Intent)
}

// partsAllowsLegacyBusinessIntentFallback is limited to continuation turns.
// Fresh requests should stay model-led even when no model-generated task
// contract is available; otherwise old keyword matchers become a hidden
// business-intent classifier again.
func partsAllowsLegacyBusinessIntentFallback(parts *chatRequestParts) bool {
	switch partsModelTurnIntentName(parts) {
	case "continue_previous_task":
		return true
	case "":
		return partsRequestsContinuationWithFallback(parts, "")
	default:
		return false
	}
}

func partsAllowsLegacyFileIntentFallback(parts *chatRequestParts) bool {
	return partsAllowsLegacyBusinessIntentFallback(parts)
}

func partsFileIntentFallbackText(parts *chatRequestParts, fallback string) string {
	if text := strings.TrimSpace(fallback); text != "" {
		return text
	}
	if parts == nil {
		return ""
	}
	return parts.Query
}

func partsRequestsManagedFileCreate(parts *chatRequestParts) bool {
	return partsRequestsManagedFileCreateWithFallback(parts, "")
}

func partsRequestsManagedFileCreateWithFallback(parts *chatRequestParts, fallback string) bool {
	switch partsModelTurnIntentName(parts) {
	case "save_generated_file_to_file_management":
		return true
	case "continue_previous_task":
		return isManagedFileCreateIntent(partsFileIntentFallbackText(parts, fallback))
	case "":
		return partsAllowsLegacyBusinessIntentFallback(parts) &&
			isManagedFileCreateIntent(partsFileIntentFallbackText(parts, fallback))
	default:
		return false
	}
}

func partsRequestsTemporaryFileGenerateWithFallback(parts *chatRequestParts, fallback string) bool {
	if parts == nil {
		return false
	}
	if modelTurnIntentRequestsTemporaryFileArtifact(parts.ModelTurnIntent) {
		return true
	}
	if !partsAllowsLegacyFileIntentFallback(parts) {
		return false
	}
	return isTemporaryFileGenerateIntent(partsFileIntentFallbackText(parts, fallback))
}

func partsRequestsFileDeleteWithFallback(parts *chatRequestParts, fallback string) bool {
	switch partsModelTurnIntentName(parts) {
	case "delete_visible_file":
		return true
	case "continue_previous_task":
		return isFileDeleteIntent(partsFileIntentFallbackText(parts, fallback))
	case "":
		return partsAllowsLegacyBusinessIntentFallback(parts) &&
			isFileDeleteIntent(partsFileIntentFallbackText(parts, fallback))
	default:
		return false
	}
}

func partsRequestsFileReadWithFallback(parts *chatRequestParts, fallback string) bool {
	intent := partsModelTurnIntentName(parts)
	if intent == "read_visible_file_content" {
		return true
	}
	if parts != nil && modelTurnIntentHasRecommendedCapability(parts.ModelTurnIntent, "visible_file_content", "file_content", "source_file_content") {
		return true
	}
	if intent != "continue_previous_task" && !(intent == "" && partsAllowsLegacyBusinessIntentFallback(parts)) {
		return false
	}
	return isFileReadIntent(partsFileIntentFallbackText(parts, fallback))
}

func partsRequestsContinuationWithFallback(parts *chatRequestParts, fallback string) bool {
	if partsModelTurnIntentName(parts) == "continue_previous_task" {
		return true
	}
	text := partsFileIntentFallbackText(parts, fallback)
	if partsModelTurnIntentName(parts) != "" {
		return isExplicitContinuationCommand(text)
	}
	return isContinuationIntent(text)
}

func partsRequestsConsoleNavigationWithFallback(parts *chatRequestParts, fallback string) bool {
	switch partsModelTurnIntentName(parts) {
	case "navigate_console_page":
		return true
	case "continue_previous_task":
		return isConsoleNavigationIntent(partsFileIntentFallbackText(parts, fallback))
	case "":
		return partsAllowsLegacyBusinessIntentFallback(parts) &&
			isConsoleNavigationIntent(partsFileIntentFallbackText(parts, fallback))
	default:
		return false
	}
}

func consoleNavigationResolvedTargetsForParts(parts *chatRequestParts) []consoleNavigationRouteHint {
	if parts == nil {
		return nil
	}
	switch partsModelTurnIntentName(parts) {
	case "navigate_console_page":
		if target := consoleNavigationRouteHintFromModelIntent(parts.ModelTurnIntent); target.Href != "" {
			return []consoleNavigationRouteHint{target}
		}
		return consoleNavigationResolvedTargets(parts.Query)
	case "continue_previous_task", "":
		return consoleNavigationResolvedTargets(parts.Query)
	default:
		return nil
	}
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
