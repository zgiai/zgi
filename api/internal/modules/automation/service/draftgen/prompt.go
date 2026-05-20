package draftgen

import (
	"encoding/json"
	"time"

	"github.com/zgiai/zgi/api/internal/prompt"
)

type promptContext struct {
	Prompt                string `json:"prompt"`
	Locale                string `json:"locale,omitempty"`
	Timezone              string `json:"timezone,omitempty"`
	CurrentTimeUTC        string `json:"current_time_utc"`
	CurrentTimeInTimezone string `json:"current_time_in_timezone,omitempty"`
}

type userPromptTemplateData struct {
	LanguageInstruction string
	ContextJSON         string
}

func buildSystemPrompt() (string, error) {
	tmpl, err := prompt.GetTemplate(prompt.AutomationTaskDraftSystem)
	if err != nil {
		return "", err
	}
	return tmpl.Render(nil)
}

func buildUserPrompt(req GenerateRequest) (string, error) {
	locale := cleanShortText(req.Locale)
	languageInstruction := "Use English for generated task names, descriptions, subjects, bodies, warnings, and missing field labels."
	if isChineseLocale(locale) {
		languageInstruction = "Use Simplified Chinese for generated task names, descriptions, subjects, bodies, warnings, and missing field labels."
	}

	now := time.Now()
	timezone := cleanShortText(req.Timezone)
	payload, err := json.MarshalIndent(promptContext{
		Prompt:                cleanLongText(req.Prompt, 4000),
		Locale:                locale,
		Timezone:              timezone,
		CurrentTimeUTC:        now.UTC().Format(time.RFC3339),
		CurrentTimeInTimezone: formatTimeInTimezone(now, timezone),
	}, "", "  ")
	if err != nil {
		return "", err
	}

	tmpl, err := prompt.GetTemplate(prompt.AutomationTaskDraftUser)
	if err != nil {
		return "", err
	}
	return tmpl.Render(userPromptTemplateData{
		LanguageInstruction: languageInstruction,
		ContextJSON:         string(payload),
	})
}

func formatTimeInTimezone(now time.Time, timezone string) string {
	if timezone == "" {
		return ""
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return ""
	}
	return now.In(location).Format(time.RFC3339)
}
