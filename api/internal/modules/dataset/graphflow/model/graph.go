package model

import "time"

// GraphEntity represents a canonical node in the knowledge graph
// Defined manually since protoc is not available in this environment
type GraphEntity struct {
	ID            string    `json:"id"`
	CanonicalName string    `json:"canonical_name"`
	Aliases       []string  `json:"aliases"`
	Description   string    `json:"description"`
	Type          string    `json:"type"` // e.g., Person, Org, Location
	Embedding     []float32 `json:"-"`    // Internal use
	Confidence    float64   `json:"confidence"`
}

// CanonicalEntity represents the resolved master entity
type CanonicalEntity struct {
	GraphEntity
	PrimaryEvidenceID string `json:"primary_evidence_id"`
}

// Mention represents a specific occurrence in a document
type Mention struct {
	ID              string          `json:"id"`
	Text            string          `json:"text"`
	DocumentID      string          `json:"document_id"`
	SegmentID       string          `json:"segment_id"`
	Position        []int           `json:"position"` // [start, end]
	Evidence        *EvidenceSource `json:"evidence"`
	CanonicalEntity *CanonicalEntity `json:"canonical_entity,omitempty"`
}

// EvidenceSource tracks data provenance with high granularity for auditability
type EvidenceSource struct {
	SourceType   string    `json:"source_type"`   // e.g., "inference", "extraction", "document"
	SourceID     string    `json:"source_id"`     // Document ID (UUID)
	OffsetStart  int       `json:"offset_start"`  // Character offset start in source text
	OffsetEnd    int       `json:"offset_end"`    // Character offset end in source text
	OriginalText string    `json:"original_text"` // The exact text snippet extracted
	Confidence   float64   `json:"confidence"`    // 0.0 - 1.0 extraction confidence
	Timestamp    time.Time `json:"timestamp"`
	ExtractorVer string    `json:"extractor_ver"` // Version of the extractor model used
}

// GraphTriple represents a validated relationship
type GraphTriple struct {
	Subject   *GraphEntity `json:"subject"`
	Predicate string       `json:"predicate"`
	Object    *GraphEntity `json:"object"`
	Weight    float64      `json:"weight"`
	Evidence  *EvidenceSource `json:"evidence"`
}

// EntityResolutionResult captures the result of an ER process
type EntityResolutionResult struct {
	Entity      *GraphEntity
	IsNew       bool
	Confidence  float64
	SourceNodes []string // IDs of source nodes merged into this canonical entity
}
