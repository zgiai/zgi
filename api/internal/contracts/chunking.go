package contracts

// ChunkUseCase describes the downstream business purpose for chunk planning.
type ChunkUseCase string

const (
	ChunkUseCaseDatasetIndex ChunkUseCase = "dataset_index"
	ChunkUseCaseChatContext  ChunkUseCase = "chat_context"
	ChunkUseCasePreview      ChunkUseCase = "preview"
	ChunkUseCaseQAIndex      ChunkUseCase = "qa_index"
)

// ChunkKind describes the semantic role of a chunk unit consumed downstream.
type ChunkKind string

const (
	ChunkKindText      ChunkKind = "text"
	ChunkKindHeading   ChunkKind = "heading"
	ChunkKindTable     ChunkKind = "table"
	ChunkKindFigure    ChunkKind = "figure"
	ChunkKindFormula   ChunkKind = "formula"
	ChunkKindParent    ChunkKind = "parent"
	ChunkKindChild     ChunkKind = "child"
	ChunkKindQuestion  ChunkKind = "qa_question"
	ChunkKindAnswer    ChunkKind = "qa_answer"
	ChunkKindReference ChunkKind = "reference"
)

// ChunkSourceElement is the canonical unit exposed by parsing capability to
// downstream chunk planning logic.
type ChunkSourceElement struct {
	ElementID  string            `json:"element_id,omitempty"`
	Type       string            `json:"type"`
	Subtype    string            `json:"subtype,omitempty"`
	Page       int               `json:"page"`
	Content    string            `json:"content,omitempty"`
	Markdown   string            `json:"markdown,omitempty"`
	BBox       *ParseBoundingBox `json:"bbox,omitempty"`
	Ordinal    int               `json:"ordinal"`
	Precision  string            `json:"precision,omitempty"`
	Confidence *float64          `json:"confidence,omitempty"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
}

// ChunkSourceDocument is the stable intermediate representation between parse
// artifact and business-specific chunk strategies.
type ChunkSourceDocument struct {
	DocumentID  string               `json:"document_id,omitempty"`
	DatasetID   string               `json:"dataset_id,omitempty"`
	FileID      string               `json:"file_id,omitempty"`
	Source      string               `json:"source,omitempty"`
	Title       string               `json:"title,omitempty"`
	Language    string               `json:"language,omitempty"`
	Elements    []ChunkSourceElement `json:"elements"`
	Metadata    map[string]any       `json:"metadata,omitempty"`
	Diagnostics map[string]any       `json:"diagnostics,omitempty"`
}

// ChunkPlan is the normalized planning output a downstream processor can use
// before producing vector/qa/preview chunk units.
type ChunkPlan struct {
	UseCase       ChunkUseCase   `json:"use_case"`
	ParentMode    string         `json:"parent_mode,omitempty"`
	Segmentation  string         `json:"segmentation,omitempty"`
	TargetKinds   []ChunkKind    `json:"target_kinds,omitempty"`
	PreserveOrder bool           `json:"preserve_order"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// ChunkUnit is the stable retrieval-facing output contract. Existing dataset
// chunkers can gradually migrate toward it without losing business metadata.
type ChunkUnit struct {
	ChunkID       string            `json:"chunk_id"`
	ParentChunkID string            `json:"parent_chunk_id,omitempty"`
	DocumentID    string            `json:"document_id,omitempty"`
	DatasetID     string            `json:"dataset_id,omitempty"`
	FileID        string            `json:"file_id,omitempty"`
	Kind          ChunkKind         `json:"kind"`
	Content       string            `json:"content"`
	Markdown      string            `json:"markdown,omitempty"`
	Pages         []int             `json:"pages,omitempty"`
	Order         int               `json:"order"`
	BBox          *ParseBoundingBox `json:"bbox,omitempty"`
	Metadata      map[string]any    `json:"metadata,omitempty"`
}

// ChunkSourceMapper converts parsing artifacts into the canonical source model.
type ChunkSourceMapper interface {
	FromParseArtifact(artifact *ParseArtifact) (*ChunkSourceDocument, error)
}

// ChunkPlanner turns a canonical document into a chunk plan for a use case.
type ChunkPlanner interface {
	Plan(doc *ChunkSourceDocument, useCase ChunkUseCase) (*ChunkPlan, error)
}
