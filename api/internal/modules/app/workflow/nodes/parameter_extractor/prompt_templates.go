package parameterextractor

import (
	"strings"

	"github.com/zgiai/ginext/internal/prompt"
)

type promptEngineeringTemplateData struct {
	Structure   string
	Instruction string
	Text        string
}

type promptEngineeringExampleTemplate struct {
	role       string
	templateID prompt.TemplateID
}

var promptEngineeringExamples = []promptEngineeringExampleTemplate{
	{role: "user", templateID: prompt.WorkflowParameterExtractorExampleUser1},
	{role: "assistant", templateID: prompt.WorkflowParameterExtractorExampleAsst1},
	{role: "user", templateID: prompt.WorkflowParameterExtractorExampleUser2},
	{role: "assistant", templateID: prompt.WorkflowParameterExtractorExampleAsst2},
}

func renderWorkflowPromptTemplate(id prompt.TemplateID, data interface{}) (string, error) {
	tmpl, err := prompt.GetTemplate(id)
	if err != nil {
		return "", err
	}

	content, err := tmpl.Render(data)
	if err != nil {
		return "", err
	}

	return strings.TrimSuffix(content, "\n"), nil
}
