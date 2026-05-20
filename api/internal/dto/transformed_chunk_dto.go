package dto

// TransformedChunk is the normalized output of document transformation.
type TransformedChunk struct {
	Content  string                  `json:"content"`
	BBox     *ExtractBoundingBox     `json:"bbox,omitempty"`
	Metadata map[string]any          `json:"metadata,omitempty"`
	Children []TransformedChildChunk `json:"children,omitempty"`
}

// TransformedChildChunk is a child chunk attached to a transformed parent chunk.
type TransformedChildChunk struct {
	Content  string              `json:"content"`
	BBox     *ExtractBoundingBox `json:"bbox,omitempty"`
	Metadata map[string]any      `json:"metadata,omitempty"`
}

// DocumentsToTransformedChunks converts legacy documents to transformed chunks.
func DocumentsToTransformedChunks(documents []Document) []TransformedChunk {
	chunks := make([]TransformedChunk, 0, len(documents))
	for _, document := range documents {
		children := make([]TransformedChildChunk, 0, len(document.Children))
		for _, child := range document.Children {
			children = append(children, TransformedChildChunk{
				Content:  child.PageContent,
				Metadata: cloneMetadata(child.Metadata),
			})
		}

		chunks = append(chunks, TransformedChunk{
			Content:  document.PageContent,
			Metadata: cloneMetadata(document.Metadata),
			Children: children,
		})
	}
	return chunks
}

// TransformedChunksToDocuments converts transformed chunks to legacy documents.
func TransformedChunksToDocuments(chunks []TransformedChunk) []Document {
	documents := make([]Document, 0, len(chunks))
	for _, chunk := range chunks {
		children := make([]ChildDocument, 0, len(chunk.Children))
		for _, child := range chunk.Children {
			children = append(children, ChildDocument{
				PageContent: child.Content,
				Metadata:    cloneMetadata(child.Metadata),
			})
		}

		documents = append(documents, Document{
			PageContent: chunk.Content,
			Metadata:    cloneMetadata(chunk.Metadata),
			Children:    children,
		})
	}
	return documents
}
