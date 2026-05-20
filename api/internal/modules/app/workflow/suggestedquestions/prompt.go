package suggestedquestions

import (
	"encoding/json"

	"github.com/zgiai/ginext/internal/prompt"
)

type userPromptTemplateData struct {
	LanguageInstruction string
	Count               int
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
		WorkflowContextJSON: string(payload),
	})
}
