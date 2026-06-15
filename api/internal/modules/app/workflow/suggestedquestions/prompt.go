package suggestedquestions

import (
	"encoding/json"

	"github.com/zgiai/zgi/api/internal/prompt"
)

type userPromptTemplateData struct {
	LanguageInstruction string
	Count               int
	ApplicationKind     string
	WorkflowContextJSON string
}

func buildSystemPrompt() (string, error) {
	tmpl, err := prompt.GetTemplate(prompt.WorkflowSuggestedQuestionsSystem)
	if err != nil {
		return "", err
	}
	return tmpl.Render(nil)
}

func buildUserPrompt(ctx WorkflowContext, count int) (string, error) {
	payload, err := json.MarshalIndent(ctx, "", "  ")
	if err != nil {
		return "", err
	}

	languageInstruction := "Use English."
	if ctx.Locale == "zh-Hans" {
		languageInstruction = "Use Simplified Chinese."
	}

	tmpl, err := prompt.GetTemplate(prompt.WorkflowSuggestedQuestionsUser)
	if err != nil {
		return "", err
	}
	return tmpl.Render(userPromptTemplateData{
		LanguageInstruction: languageInstruction,
		Count:               count,
		ApplicationKind:     applicationKind(ctx.WorkflowType),
		WorkflowContextJSON: string(payload),
	})
}

func applicationKind(workflowType string) string {
	switch workflowType {
	case "AGENT":
		return "agent web app"
	case "WORKFLOW", "CONVERSATIONAL_WORKFLOW":
		return "workflow web app"
	default:
		return "AI app"
	}
}
