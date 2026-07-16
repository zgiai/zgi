package service

import (
	"sort"
	"strings"
	"time"
)

const runtimeTimelineMetadataKey = "runtime_timeline"

func ensureInvocationTimelineTime(invocation map[string]interface{}, occurredAt time.Time) {
	if len(invocation) == 0 {
		return
	}
	if _, ok := unixMillisecondsFromAny(invocation["created_at_ms"]); ok {
		normalizeSkillInvocationTimelineFields(invocation)
		return
	}
	if _, ok := unixSecondsFromAny(invocation["created_at"]); ok {
		normalizeSkillInvocationTimelineFields(invocation)
		return
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}
	invocation["created_at"] = occurredAt.Unix()
	invocation["created_at_ms"] = occurredAt.UnixMilli()
}

func upsertRuntimeTimeline(metadata map[string]interface{}, invocation map[string]interface{}, source string) {
	if metadata == nil || len(invocation) == 0 {
		return
	}
	runtimeID := strings.TrimSpace(stringFromAny(invocation["runtime_id"]))
	if runtimeID == "" {
		return
	}
	createdAtMS, _ := skillInvocationTimelineMilliseconds(invocation)
	timeline := runtimeTimelineFromMetadata(metadata[runtimeTimelineMetadataKey])
	if len(timeline) == 0 {
		timeline = bootstrapRuntimeTimeline(metadata)
	}
	for index, entry := range timeline {
		if strings.TrimSpace(stringFromAny(entry["runtime_id"])) != runtimeID {
			continue
		}
		sequence := runtimeTimelineInt64(entry["sequence"])
		if sequence > 0 {
			invocation["sequence"] = sequence
		}
		if existingAt, ok := unixMillisecondsFromAny(entry["created_at_ms"]); ok && existingAt > 0 {
			invocation["created_at_ms"] = existingAt
			invocation["created_at"] = existingAt / 1000
			createdAtMS = existingAt
		}
		timeline[index] = compactSkillInvocation(map[string]interface{}{
			"runtime_id":    runtimeID,
			"kind":          invocation["kind"],
			"source":        source,
			"source_id":     runtimeTimelineSourceID(invocation, source),
			"created_at_ms": createdAtMS,
			"sequence":      sequence,
		})
		metadata[runtimeTimelineMetadataKey] = runtimeTimelineToInterfaceSlice(timeline)
		return
	}
	sequence := nextRuntimeTimelineSequence(timeline)
	invocation["sequence"] = sequence
	timeline = append(timeline, compactSkillInvocation(map[string]interface{}{
		"runtime_id":    runtimeID,
		"kind":          invocation["kind"],
		"source":        source,
		"source_id":     runtimeTimelineSourceID(invocation, source),
		"created_at_ms": createdAtMS,
		"sequence":      sequence,
	}))
	metadata[runtimeTimelineMetadataKey] = runtimeTimelineToInterfaceSlice(timeline)
}

func bootstrapRuntimeTimeline(metadata map[string]interface{}) []map[string]interface{} {
	models := sortRuntimeTimelineInvocations(modelInvocationsFromMetadata(metadata["model_invocations"]))
	activities := sortRuntimeTimelineInvocations(skillInvocationsFromMetadata(metadata["skill_invocations"]))
	ordered := make([]runtimeTimelineSourceInvocation, 0, len(models)+len(activities))
	usedActivities := make([]bool, len(activities))
	for _, modelInvocation := range models {
		ordered = append(ordered, runtimeTimelineSourceInvocation{source: "model_invocations", invocation: modelInvocation})
		for _, toolName := range runtimeTimelineModelToolCallNames(modelInvocation) {
			for activityIndex, activity := range activities {
				if usedActivities[activityIndex] || !runtimeTimelineToolMatchesInvocation(toolName, activity) {
					continue
				}
				ordered = append(ordered, runtimeTimelineSourceInvocation{source: "skill_invocations", invocation: activity})
				usedActivities[activityIndex] = true
				break
			}
		}
	}
	for activityIndex, activity := range activities {
		if !usedActivities[activityIndex] {
			ordered = append(ordered, runtimeTimelineSourceInvocation{source: "skill_invocations", invocation: activity})
		}
	}
	ordered = insertLegacyWorkflowTimelineInvocations(ordered, workflowRunsFromMetadata(metadata["workflow_runs"]))

	timeline := make([]map[string]interface{}, 0, len(ordered))
	for _, item := range ordered {
		runtimeID := strings.TrimSpace(stringFromAny(item.invocation["runtime_id"]))
		if runtimeID == "" {
			continue
		}
		createdAtMS, _ := skillInvocationTimelineMilliseconds(item.invocation)
		sequence := int64(len(timeline) + 1)
		item.invocation["sequence"] = sequence
		sourceID := strings.TrimSpace(item.sourceID)
		if sourceID == "" {
			sourceID = runtimeID
		}
		timeline = append(timeline, compactSkillInvocation(map[string]interface{}{
			"runtime_id":    runtimeID,
			"kind":          item.invocation["kind"],
			"source":        item.source,
			"source_id":     sourceID,
			"created_at_ms": createdAtMS,
			"sequence":      sequence,
		}))
	}
	return timeline
}

type runtimeTimelineSourceInvocation struct {
	source     string
	sourceID   string
	invocation map[string]interface{}
}

func insertLegacyWorkflowTimelineInvocations(ordered []runtimeTimelineSourceInvocation, runs []map[string]interface{}) []runtimeTimelineSourceInvocation {
	if len(runs) == 0 {
		return ordered
	}
	workflowItems := make([]runtimeTimelineSourceInvocation, 0, len(runs))
	for _, run := range runs {
		runID := strings.TrimSpace(stringFromAny(run["workflow_run_id"]))
		if runID == "" {
			continue
		}
		invocation := map[string]interface{}{
			"kind":       "workflow_run",
			"runtime_id": "workflow_run:" + runID,
		}
		if createdAt := numericValueFromAny(run["created_at"]); createdAt != nil {
			invocation["created_at"] = createdAt
		}
		if createdAtMS := numericValueFromAny(run["created_at_ms"]); createdAtMS != nil {
			invocation["created_at_ms"] = createdAtMS
		}
		normalizeSkillInvocationTimelineFields(invocation)
		workflowItems = append(workflowItems, runtimeTimelineSourceInvocation{
			source:     "workflow_runs",
			sourceID:   runID,
			invocation: invocation,
		})
	}
	if len(workflowItems) == 0 {
		return ordered
	}
	expanded := make([]runtimeTimelineSourceInvocation, 0, len(ordered)+len(workflowItems))
	workflowIndex := 0
	for _, item := range ordered {
		expanded = append(expanded, item)
		if workflowIndex >= len(workflowItems) || item.source != "skill_invocations" {
			continue
		}
		if strings.TrimSpace(stringFromAny(item.invocation["skill_id"])) != "agent-workflow" &&
			strings.TrimSpace(stringFromAny(item.invocation["tool_name"])) != "run_agent_workflow" {
			continue
		}
		expanded = append(expanded, workflowItems[workflowIndex])
		workflowIndex++
	}
	return append(expanded, workflowItems[workflowIndex:]...)
}

func sortRuntimeTimelineInvocations(invocations []map[string]interface{}) []map[string]interface{} {
	sort.SliceStable(invocations, func(i, j int) bool {
		left, leftOK := skillInvocationTimelineMilliseconds(invocations[i])
		right, rightOK := skillInvocationTimelineMilliseconds(invocations[j])
		if leftOK != rightOK {
			return leftOK
		}
		if !leftOK || left == right {
			return false
		}
		return left < right
	})
	return invocations
}

func runtimeTimelineModelToolCallNames(invocation map[string]interface{}) []string {
	response := mapFromOperationContext(invocation["response"])
	if names := stringSliceFromAny(response["tool_call_names"]); len(names) > 0 {
		return names
	}
	message := mapFromOperationContext(response["message"])
	calls := mapSliceFromAny(message["tool_calls"])
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		function := mapFromOperationContext(call["function"])
		names = append(names, strings.TrimSpace(stringFromAny(function["name"])))
	}
	return names
}

func runtimeTimelineToolMatchesInvocation(toolName string, invocation map[string]interface{}) bool {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["tool_name"])), toolName) {
		return true
	}
	switch strings.TrimSpace(stringFromAny(invocation["kind"])) {
	case "user_input_request":
		return toolName == "request_user_input"
	case "skill_load":
		return toolName == "load_skill"
	default:
		return false
	}
}

func upsertWorkflowRuntimeTimeline(metadata map[string]interface{}, run map[string]interface{}) {
	if metadata == nil || len(run) == 0 {
		return
	}
	runID := strings.TrimSpace(stringFromAny(run["workflow_run_id"]))
	if runID == "" {
		return
	}
	invocation := map[string]interface{}{
		"runtime_id": "workflow_run:" + runID,
		"kind":       "workflow_run",
		"source_id":  runID,
	}
	if createdAt := numericValueFromAny(run["created_at"]); createdAt != nil {
		invocation["created_at"] = createdAt
	}
	if createdAtMS := numericValueFromAny(run["created_at_ms"]); createdAtMS != nil {
		invocation["created_at_ms"] = createdAtMS
	}
	ensureInvocationTimelineTime(invocation, time.Now())
	upsertRuntimeTimeline(metadata, invocation, "workflow_runs")
	if sequence := runtimeTimelineInt64(invocation["sequence"]); sequence > 0 {
		run["sequence"] = sequence
	}
	if createdAtMS, ok := unixMillisecondsFromAny(invocation["created_at_ms"]); ok {
		run["created_at_ms"] = createdAtMS
		run["created_at"] = createdAtMS / 1000
	}
}

func runtimeTimelineSourceID(invocation map[string]interface{}, source string) string {
	if source == "workflow_runs" {
		if sourceID := strings.TrimSpace(stringFromAny(invocation["source_id"])); sourceID != "" {
			return sourceID
		}
		return strings.TrimPrefix(strings.TrimSpace(stringFromAny(invocation["runtime_id"])), "workflow_run:")
	}
	return strings.TrimSpace(stringFromAny(invocation["runtime_id"]))
}

func nextRuntimeTimelineSequence(timeline []map[string]interface{}) int64 {
	var maximum int64
	for _, entry := range timeline {
		if sequence := runtimeTimelineInt64(entry["sequence"]); sequence > maximum {
			maximum = sequence
		}
	}
	return maximum + 1
}

func runtimeTimelineFromMetadata(value interface{}) []map[string]interface{} {
	timeline := skillInvocationsFromMetadata(value)
	sort.SliceStable(timeline, func(i, j int) bool {
		return runtimeTimelineInt64(timeline[i]["sequence"]) < runtimeTimelineInt64(timeline[j]["sequence"])
	})
	return timeline
}

func runtimeTimelineToInterfaceSlice(timeline []map[string]interface{}) []interface{} {
	out := make([]interface{}, 0, len(timeline))
	for _, entry := range timeline {
		out = append(out, entry)
	}
	return out
}

func runtimeTimelineInt64(value interface{}) int64 {
	if parsed, ok := unixMillisecondsFromAny(value); ok {
		return parsed
	}
	return 0
}
