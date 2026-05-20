package sqlgenerator

import (
	"strings"

	"github.com/zgiai/ginext/internal/prompt"
)

const (
	systemPromptSourceEmbeddedDefault = "embedded_default"
	systemPromptSourceNodeOverride    = "node_override"
)

type resolvedSystemPrompt struct {
	Content string
	Source  string
}

type systemPromptTemplateData struct{}

func (n *Node) resolveSystemPrompt() (*resolvedSystemPrompt, error) {
	if strings.TrimSpace(n.NodeData.Prompt.System) != "" {
		return &resolvedSystemPrompt{
			Content: n.NodeData.Prompt.System,
			Source:  systemPromptSourceNodeOverride,
		}, nil
	}

	tmpl, err := prompt.GetTemplate(prompt.WorkflowSQLGeneratorSystem)
	if err != nil {
		return nil, err
	}

	content, err := tmpl.Render(systemPromptTemplateData{})
	if err != nil {
		return nil, err
	}

	return &resolvedSystemPrompt{
		Content: content,
		Source:  systemPromptSourceEmbeddedDefault,
	}, nil
}
