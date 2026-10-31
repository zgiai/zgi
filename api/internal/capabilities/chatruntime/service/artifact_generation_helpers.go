package service

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

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
