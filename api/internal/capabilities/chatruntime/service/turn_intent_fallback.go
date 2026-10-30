package service

func partsModelTurnIntentName(parts *chatRequestParts) string {
	if parts == nil || parts.ModelTurnIntent == nil {
		return ""
	}
	return normalizeModelTurnIntent(parts.ModelTurnIntent.Intent)
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
