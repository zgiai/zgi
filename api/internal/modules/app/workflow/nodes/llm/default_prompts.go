package llm

import (
	"strings"

	"github.com/zgiai/ginext/internal/prompt"
)

const (
	defaultChatSystemPromptFallback = "You are a helpful AI assistant."
	defaultCompletionPromptFallback = "Here are the chat histories between human and assistant, inside <histories></histories> XML tags.\n\n<histories>\n{{#histories#}}\n</histories>\n\n\nHuman: {{#sys.query#}}\n\nAssistant:"
)

type llmChatSystemDefaultTemplateData struct{}

type llmCompletionDefaultTemplateData struct {
	HistoriesPlaceholder string
	SysQueryPlaceholder  string
}

var getPromptTemplate = prompt.GetTemplate

func renderDefaultChatSystemPrompt() string {
	content, err := renderWorkflowDefaultPrompt(prompt.WorkflowLLMChatSystemDefault, llmChatSystemDefaultTemplateData{})
	if err != nil {
		return defaultChatSystemPromptFallback
	}
	return content
}

func renderDefaultCompletionPrompt() string {
	content, err := renderWorkflowDefaultPrompt(
		prompt.WorkflowLLMCompletionDefault,
		llmCompletionDefaultTemplateData{
			HistoriesPlaceholder: "{{#histories#}}",
			SysQueryPlaceholder:  "{{#sys.query#}}",
		},
	)
	if err != nil {
		return defaultCompletionPromptFallback
	}
	return content
}

func renderWorkflowDefaultPrompt(id prompt.TemplateID, data interface{}) (string, error) {
	tmpl, err := getPromptTemplate(id)
	if err != nil {
		return "", err
	}

	content, err := tmpl.Render(data)
	if err != nil {
		return "", err
	}

	return strings.TrimSuffix(content, "\n"), nil
}
