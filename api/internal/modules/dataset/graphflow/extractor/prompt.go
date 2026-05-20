package extractor

import "github.com/zgiai/ginext/internal/prompt"

const (
	defaultGlobalContextValue = "No specific global context provided."
	defaultDocumentTitleValue = "Untitled Document"
)

type globalEntitySummaryPromptData struct {
	DocumentText string
}

type graphExtractionPromptData struct {
	GlobalContext string
	DocumentTitle string
	SegmentText   string
}

func renderGlobalEntitySummaryPrompt(documentText string) (string, error) {
	tmpl, err := prompt.GetTemplate(prompt.GraphFlowGlobalEntitySummary)
	if err != nil {
		return "", err
	}
	return tmpl.Render(globalEntitySummaryPromptData{DocumentText: documentText})
}

func renderGraphExtractionPrompt(globalContext, documentTitle, segmentText string) (string, error) {
	tmpl, err := prompt.GetTemplate(prompt.GraphFlowGraphExtraction)
	if err != nil {
		return "", err
	}
	return tmpl.Render(graphExtractionPromptData{
		GlobalContext: globalContext,
		DocumentTitle: documentTitle,
		SegmentText:   segmentText,
	})
}
