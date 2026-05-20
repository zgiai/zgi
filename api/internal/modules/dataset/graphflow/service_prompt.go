package graphflow

import "github.com/zgiai/ginext/internal/prompt"

type queryEntityExtractionPromptData struct {
	Query string
}

func renderQueryEntityExtractionPrompt(query string) (string, error) {
	tmpl, err := prompt.GetTemplate(prompt.GraphFlowQueryEntityExtraction)
	if err != nil {
		return "", err
	}
	return tmpl.Render(queryEntityExtractionPromptData{Query: query})
}
