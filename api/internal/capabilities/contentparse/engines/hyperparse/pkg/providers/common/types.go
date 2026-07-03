package common

// DocumentResult is the canonical document-level result returned by all parse engines.
type DocumentResult struct {
	DocID       string            `json:"doc_id"`
	FileName    string            `json:"file_name"`
	PageCount   int               `json:"page_count"`
	Pages       []Page            `json:"pages,omitempty"`
	Chunks      []Chunk           `json:"chunks,omitempty"`
	Markdown    string            `json:"markdown,omitempty"`
	Source      string            `json:"source,omitempty"`
	Diagnostics map[string]any    `json:"diagnostics,omitempty"`
	ImageAssets map[string]string `json:"-"`
	// ExtractOutput is the default structured output for downstream ETL-style callers.
	ExtractOutput *ExtractOutput `json:"extract_output,omitempty"`
}

type BBox struct {
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
}

type Page struct {
	PageIndex int     `json:"page_index"`
	Width     float64 `json:"width,omitempty"`
	Height    float64 `json:"height,omitempty"`
	ImageURL  string  `json:"image_url,omitempty"`
}

type Chunk struct {
	ID         string         `json:"id"`
	ParentID   string         `json:"parent_id,omitempty"`
	Type       string         `json:"type"`
	Subtype    string         `json:"subtype,omitempty"`
	Page       int            `json:"page"`
	BBox       *BBox          `json:"bbox,omitempty"`
	Text       string         `json:"text,omitempty"`
	Markdown   string         `json:"markdown,omitempty"`
	Ordinal    int            `json:"ordinal,omitempty"`
	Confidence float64        `json:"confidence,omitempty"`
	Precision  string         `json:"precision,omitempty"`
	Payload    map[string]any `json:"payload,omitempty"`
}

// ExtractOutput is the normalized output downstream services can consume directly.
type ExtractOutput struct {
	Markdown string           `json:"markdown,omitempty"`
	Elements []ExtractElement `json:"elements,omitempty"`
	Source   string           `json:"source,omitempty"`
	Metadata map[string]any   `json:"metadata,omitempty"`
}

// ExtractElement is an independently consumable layout element.
type ExtractElement struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Subtype   string         `json:"subtype,omitempty"`
	Page      int            `json:"page"`
	Content   string         `json:"content,omitempty"`
	BBox      *BBox          `json:"bbox,omitempty"`
	Ordinal   int            `json:"ordinal"`
	Precision string         `json:"precision,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// EnrichStructuredOutput fills ExtractOutput from DocumentResult when it is absent.
// Rules:
// 1) each chunk maps to one element;
// 2) ordinal defines reading order, falling back to appearance order;
// 3) markdown, parent_id, confidence, and payload are preserved in element metadata.
func EnrichStructuredOutput(doc *DocumentResult) *DocumentResult {
	if doc == nil {
		return nil
	}
	if doc.ExtractOutput != nil {
		return doc
	}

	elements := make([]ExtractElement, 0, len(doc.Chunks))
	for i, ch := range doc.Chunks {
		ord := ch.Ordinal
		if ord <= 0 {
			ord = i + 1
		}
		el := ExtractElement{
			ID:        ch.ID,
			Type:      ch.Type,
			Subtype:   ch.Subtype,
			Page:      ch.Page,
			Content:   ch.Text,
			BBox:      ch.BBox,
			Ordinal:   ord,
			Precision: ch.Precision,
		}
		meta := map[string]any{}
		if ch.Markdown != "" {
			meta["markdown"] = ch.Markdown
		}
		if ch.ParentID != "" {
			meta["parent_id"] = ch.ParentID
		}
		if ch.Confidence > 0 {
			meta["confidence"] = ch.Confidence
		}
		if len(ch.Payload) > 0 {
			meta["payload"] = ch.Payload
		}
		if len(meta) > 0 {
			el.Metadata = meta
		}
		elements = append(elements, el)
	}

	meta := map[string]any{
		"doc_id":     doc.DocID,
		"file_name":  doc.FileName,
		"page_count": doc.PageCount,
	}
	doc.ExtractOutput = &ExtractOutput{
		Markdown: doc.Markdown,
		Elements: elements,
		Source:   doc.Source,
		Metadata: meta,
	}
	return doc
}
