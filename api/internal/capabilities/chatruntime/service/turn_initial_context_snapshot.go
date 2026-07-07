package service

import "strings"

const (
	turnInitialContextSnapshotKey    = "turn_initial_context_snapshot"
	turnInitialContextSnapshotSchema = "zgi.aichat.turn_initial_context_snapshot.v1"
)

func turnInitialContextSnapshot(parts *chatRequestParts) map[string]interface{} {
	if parts == nil {
		return nil
	}
	snapshot := map[string]interface{}{
		"schema":  turnInitialContextSnapshotSchema,
		"surface": normalizeAIChatSurface(parts.Surface),
	}
	if route := initialContextRoute(parts); route != "" {
		snapshot["route"] = route
	}
	if page := initialContextPage(parts); page != "" {
		snapshot["page"] = page
	}
	if parts.RuntimeContext != "" {
		snapshot["runtime_context"] = map[string]interface{}{
			"included":   true,
			"char_count": len([]rune(parts.RuntimeContext)),
		}
	}
	if parts.OperationLedger != nil {
		snapshot["operation_ledger"] = copyStringAnyMap(parts.OperationLedger)
	}
	if len(snapshot) <= 2 {
		return nil
	}
	return snapshot
}

func restoreTurnInitialContextFromMetadata(parts *chatRequestParts, metadata map[string]interface{}) {
	if parts == nil || len(metadata) == 0 {
		return
	}
	snapshot := mapFromOperationContext(metadata[turnInitialContextSnapshotKey])
	if len(snapshot) == 0 || !strings.EqualFold(strings.TrimSpace(stringFromAny(snapshot["schema"])), turnInitialContextSnapshotSchema) {
		return
	}
	if strings.TrimSpace(parts.Surface) == "" {
		parts.Surface = normalizeAIChatSurface(stringFromAny(snapshot["surface"]))
	}
	if parts.OperationLedger == nil {
		if ledger := mapFromOperationContext(snapshot["operation_ledger"]); len(ledger) > 0 {
			parts.OperationLedger = copyStringAnyMap(ledger)
		}
	}
	if strings.TrimSpace(parts.RuntimeContext) != "" {
		return
	}
	route := strings.TrimSpace(stringFromAny(snapshot["route"]))
	page := strings.TrimSpace(stringFromAny(snapshot["page"]))
	if route == "" && page == "" && parts.OperationLedger == nil {
		return
	}
	var builder strings.Builder
	builder.WriteString("Restored send-time ZGI page context from the original AIChat turn.")
	if page != "" {
		builder.WriteString(" page=")
		builder.WriteString(page)
	}
	if route != "" {
		builder.WriteString(" route=")
		builder.WriteString(route)
	}
	if parts.OperationLedger != nil {
		if resourceCount := stringFromAny(parts.OperationLedger["resource_count"]); resourceCount != "" {
			builder.WriteString(" resources=")
			builder.WriteString(resourceCount)
		}
		if capabilityCount := stringFromAny(parts.OperationLedger["capability_count"]); capabilityCount != "" {
			builder.WriteString(" capabilities=")
			builder.WriteString(capabilityCount)
		}
	}
	parts.RuntimeContext = builder.String()
}

func initialContextRoute(parts *chatRequestParts) string {
	if parts == nil {
		return ""
	}
	if route := consoleRouteFromRuntimeContext(parts.RuntimeContext); route != "" {
		return route
	}
	for _, source := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		for _, resource := range mapSliceFromAny(source["resources"]) {
			metadata := mapFromOperationContext(resource["metadata"])
			if route := strings.TrimSpace(firstNonEmptyString(resource["href"], resource["route"], metadata["route"], metadata["href"])); route != "" {
				return route
			}
		}
	}
	return ""
}

func initialContextPage(parts *chatRequestParts) string {
	if parts == nil {
		return ""
	}
	for _, source := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		for _, resource := range mapSliceFromAny(source["resources"]) {
			metadata := mapFromOperationContext(resource["metadata"])
			if page := strings.TrimSpace(firstNonEmptyString(metadata["page"], resource["page"])); page != "" {
				return page
			}
		}
	}
	return ""
}
