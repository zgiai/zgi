package service

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func appendArtifactProducerSkills(values []string, parts *chatRequestParts) []string {
	if parts == nil {
		return values
	}
	fileEnabled := skillIDEnabled(parts.SkillIDs, skills.SkillFileGenerator)
	chartEnabled := skillIDEnabled(parts.SkillIDs, skills.SkillChartGenerator)
	if chartEnabled && shouldPreferChartArtifactProducer(parts) {
		values = appendUniqueStrings(values, skills.SkillChartGenerator)
		if fileEnabled {
			values = appendUniqueStrings(values, skills.SkillFileGenerator)
		}
		return values
	}
	if fileEnabled {
		values = appendUniqueStrings(values, skills.SkillFileGenerator)
	}
	if chartEnabled {
		values = appendUniqueStrings(values, skills.SkillChartGenerator)
	}
	return values
}

func removeArtifactProducerSkills(values []string) []string {
	if len(values) == 0 {
		return values
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || value == skills.SkillFileGenerator || value == skills.SkillChartGenerator {
			continue
		}
		out = appendUniqueStrings(out, value)
	}
	return out
}

func modelTurnIntentRequestsTemporaryFileArtifact(intent *AIChatModelTurnIntent) bool {
	if intent == nil {
		return false
	}
	if normalizeModelTurnIntent(intent.Intent) == "generate_temporary_file_artifact" {
		return true
	}
	return modelTurnIntentHasRecommendedCapability(
		intent,
		"generated_artifact",
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

func shouldPreferChartArtifactProducer(parts *chatRequestParts) bool {
	if parts == nil {
		return false
	}
	if parts.ModelTurnIntent != nil {
		switch {
		case modelTurnIntentHasRecommendedCapability(parts.ModelTurnIntent, "chart_artifact", "data_visualization_artifact", "visualization_artifact"):
			return true
		case modelTurnIntentHasRecommendedCapability(parts.ModelTurnIntent, "file_artifact", "document_artifact", "svg_artifact", "text_artifact", "pdf_artifact", "spreadsheet_artifact"):
			return false
		}
	}
	return false
}

func modelTurnIntentHasRecommendedCapability(intent *AIChatModelTurnIntent, values ...string) bool {
	if intent == nil || len(values) == 0 {
		return false
	}
	want := map[string]struct{}{}
	for _, value := range values {
		if canonical := canonicalModelTurnCapabilityHint(value); canonical != "" {
			want[canonical] = struct{}{}
		}
	}
	if len(want) == 0 {
		return false
	}
	for _, capability := range intent.RecommendedCapabilities {
		canonical := canonicalModelTurnCapabilityHint(capability)
		if _, ok := want[canonical]; ok {
			return true
		}
	}
	return false
}

func canonicalModelTurnCapabilityHint(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("-", "_", " ", "_").Replace(value)
	return value
}

func finalAnswerGuardHasSuccessfulArtifactProducerTool(req skillloop.FinalAnswerGuardRequest) bool {
	for _, call := range req.SuccessfulToolCalls {
		if isKnownArtifactGeneratorToolCall(call.SkillID, call.ToolName) {
			return true
		}
	}
	return false
}

func finalAnswerGuardHasAttemptedArtifactProducerTool(req skillloop.FinalAnswerGuardRequest) bool {
	for _, call := range req.AttemptedToolCalls {
		if isKnownArtifactGeneratorToolCall(call.SkillID, call.ToolName) {
			return true
		}
	}
	return false
}

func isFileGeneratorToolCall(skillID string, toolName string) bool {
	if !strings.EqualFold(strings.TrimSpace(skillID), skills.SkillFileGenerator) {
		return false
	}
	switch strings.TrimSpace(toolName) {
	case "generate_file", "generate_docx", "generate_pdf", "generate_pptx":
		return true
	default:
		return false
	}
}

func isChartGeneratorToolCall(skillID string, toolName string) bool {
	return strings.EqualFold(strings.TrimSpace(skillID), skills.SkillChartGenerator) &&
		strings.EqualFold(strings.TrimSpace(toolName), "generate_chart")
}

func isKnownArtifactGeneratorToolCall(skillID string, toolName string) bool {
	return isFileGeneratorToolCall(skillID, toolName) || isChartGeneratorToolCall(skillID, toolName)
}

func toolCallResultLooksLikeGeneratedArtifact(call skillloop.SkillToolCallRef) bool {
	if len(call.Result) == 0 {
		return false
	}
	if strings.TrimSpace(stringFromAny(call.Result["upload_file_id"])) != "" ||
		strings.EqualFold(strings.TrimSpace(stringFromAny(call.Result["target"])), "managed_file") {
		return false
	}
	if strings.TrimSpace(stringFromAny(call.Result["tool_file_id"])) != "" {
		return true
	}
	if isKnownArtifactGeneratorToolCall(call.SkillID, call.ToolName) &&
		strings.TrimSpace(stringFromAny(call.Result["file_id"])) != "" {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(stringFromAny(call.Result["transfer_method"])), "tool_file") {
		return true
	}
	if !hasGeneratedArtifactURL(call.Result) {
		return false
	}
	if strings.TrimSpace(stringFromAny(call.Result["file_id"])) == "" {
		return false
	}
	return strings.TrimSpace(firstNonEmptyString(
		call.Result["filename"],
		call.Result["name"],
		call.Result["mime_type"],
		call.Result["format"],
	)) != ""
}

func hasGeneratedArtifactURL(result map[string]interface{}) bool {
	return strings.TrimSpace(firstNonEmptyString(result["download_url"], result["url"])) != ""
}
