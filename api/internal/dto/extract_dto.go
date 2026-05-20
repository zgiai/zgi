package dto

import "strings"

// ExtractOutput is the normalized output of document extraction.
type ExtractOutput struct {
	Elements []ExtractElement `json:"elements"`
	Markdown string           `json:"markdown,omitempty"`
	Source   string           `json:"source,omitempty"`
	Metadata map[string]any   `json:"metadata,omitempty"`
}

// ExtractElement represents one extracted layout element.
type ExtractElement struct {
	Type      string              `json:"type"`
	Subtype   string              `json:"subtype,omitempty"`
	Page      int                 `json:"page"`
	Content   string              `json:"content,omitempty"`
	BBox      *ExtractBoundingBox `json:"bbox,omitempty"`
	Ordinal   int                 `json:"ordinal"`
	Precision string              `json:"precision,omitempty"`
	Metadata  map[string]any      `json:"metadata,omitempty"`
}

// ExtractBoundingBox represents a normalized element bounding box.
type ExtractBoundingBox struct {
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
}

// NewExtractOutputFromDocuments converts legacy page documents to extraction output.
func NewExtractOutputFromDocuments(source string, documents []Document) *ExtractOutput {
	return NewExtractOutputFromDocumentsWithType(source, "text", documents)
}

// NewExtractOutputFromDocumentsWithType converts legacy documents using one element type.
func NewExtractOutputFromDocumentsWithType(source, elementType string, documents []Document) *ExtractOutput {
	output := &ExtractOutput{
		Elements: make([]ExtractElement, 0, len(documents)),
		Source:   source,
	}

	var markdown strings.Builder
	for i, document := range documents {
		content := strings.TrimSpace(document.PageContent)
		if content == "" {
			continue
		}
		if markdown.Len() > 0 {
			markdown.WriteString("\n")
		}
		markdown.WriteString(content)

		metadata := cloneMetadata(document.Metadata)
		if document.Provider != "" {
			metadata["provider"] = document.Provider
		}

		output.Elements = append(output.Elements, ExtractElement{
			Type:     elementType,
			Page:     metadataPage(metadata),
			Content:  content,
			Ordinal:  i,
			Metadata: metadata,
		})
	}

	output.Markdown = markdown.String()
	return output
}

// ExtractOutputToDocuments converts extraction output back to legacy documents.
func ExtractOutputToDocuments(output *ExtractOutput) []Document {
	if output == nil {
		return nil
	}

	documents := make([]Document, 0, len(output.Elements))
	for _, element := range output.Elements {
		content := strings.TrimSpace(element.Content)
		if content == "" {
			continue
		}

		metadata := cloneMetadata(element.Metadata)
		if _, ok := metadata["page"]; !ok {
			metadata["page"] = element.Page
		}
		if _, ok := metadata["element_type"]; !ok && element.Type != "" {
			metadata["element_type"] = element.Type
		}
		if _, ok := metadata["source"]; !ok && output.Source != "" {
			metadata["source"] = output.Source
		}

		documents = append(documents, Document{
			PageContent: content,
			Metadata:    metadata,
			Provider:    output.Source,
		})
	}

	if len(documents) == 0 && strings.TrimSpace(output.Markdown) != "" {
		documents = append(documents, Document{
			PageContent: strings.TrimSpace(output.Markdown),
			Metadata:    cloneMetadata(output.Metadata),
			Provider:    output.Source,
		})
	}

	return documents
}

// ExtractOutputText returns markdown when available, then falls back to element content.
func ExtractOutputText(output *ExtractOutput) string {
	if output == nil {
		return ""
	}
	if text := strings.TrimSpace(output.Markdown); text != "" {
		return text
	}

	contents := make([]string, 0, len(output.Elements))
	for _, element := range output.Elements {
		if content := strings.TrimSpace(element.Content); content != "" {
			contents = append(contents, content)
		}
	}
	return strings.Join(contents, "\n")
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func metadataPage(metadata map[string]any) int {
	value, ok := metadata["page"]
	if !ok {
		return 0
	}

	switch page := value.(type) {
	case int:
		return page
	case int64:
		return int(page)
	case float64:
		return int(page)
	case float32:
		return int(page)
	default:
		return 0
	}
}
