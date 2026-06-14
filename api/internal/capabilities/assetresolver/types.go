// Package assetresolver grounds page and conversation asset references to
// stable asset IDs without depending on action runtime or tool governance.
package assetresolver

// Status describes whether a selector could be grounded to concrete assets.
type Status string

const (
	StatusResolved    Status = "resolved"
	StatusAmbiguous   Status = "ambiguous"
	StatusNotFound    Status = "not_found"
	StatusUnsupported Status = "unsupported"
)

const (
	AssetTypeFile = "file"

	defaultCandidateLimit = 8
)

// Request contains the bounded context used to resolve selectors.
type Request struct {
	OperationContext           map[string]interface{} `json:"operation_context,omitempty"`
	NormalizedOperationContext map[string]interface{} `json:"normalized_operation_context,omitempty"`
	Candidates                 []Candidate            `json:"candidates,omitempty"`
	Selectors                  []Selector             `json:"selectors,omitempty"`
	CandidateLimit             int                    `json:"candidate_limit,omitempty"`
}

// Candidate is a file-like asset visible or selected in the current AIChat turn.
type Candidate struct {
	Type           string                 `json:"type,omitempty"`
	ID             string                 `json:"id,omitempty"`
	Name           string                 `json:"name,omitempty"`
	Title          string                 `json:"title,omitempty"`
	Source         string                 `json:"source,omitempty"`
	Extension      string                 `json:"extension,omitempty"`
	MimeType       string                 `json:"mime_type,omitempty"`
	FileType       string                 `json:"file_type,omitempty"`
	WorkspaceID    string                 `json:"workspace_id,omitempty"`
	Selected       bool                   `json:"selected,omitempty"`
	Visible        bool                   `json:"visible,omitempty"`
	VisibleOrdinal int                    `json:"visible_ordinal,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// Selector describes a user or model reference such as selected file, fourth
// visible file, second Excel file, or a fuzzy filename.
type Selector struct {
	ResourceType  string                 `json:"resource_type,omitempty"`
	Type          string                 `json:"type,omitempty"`
	Kind          string                 `json:"kind,omitempty"`
	ID            string                 `json:"id,omitempty"`
	FileID        string                 `json:"file_id,omitempty"`
	Source        string                 `json:"source,omitempty"`
	Selector      string                 `json:"selector,omitempty"`
	Scope         string                 `json:"scope,omitempty"`
	Selected      bool                   `json:"selected,omitempty"`
	Ordinal       int                    `json:"ordinal,omitempty"`
	VisibleIndex  int                    `json:"visible_index,omitempty"`
	OrdinalText   string                 `json:"ordinal_text,omitempty"`
	Title         string                 `json:"title,omitempty"`
	Name          string                 `json:"name,omitempty"`
	TitleContains string                 `json:"title_contains,omitempty"`
	NameContains  string                 `json:"name_contains,omitempty"`
	FuzzyName     string                 `json:"fuzzy_name,omitempty"`
	Extension     string                 `json:"extension,omitempty"`
	Extensions    []string               `json:"extensions,omitempty"`
	MimeType      string                 `json:"mime_type,omitempty"`
	MimeTypes     []string               `json:"mime_types,omitempty"`
	FileType      string                 `json:"file_type,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// Asset is the stable resolver output used by governance and runtime adapters.
type Asset struct {
	Type        string                 `json:"type,omitempty"`
	ID          string                 `json:"id,omitempty"`
	Name        string                 `json:"name,omitempty"`
	WorkspaceID string                 `json:"workspace_id,omitempty"`
	Source      string                 `json:"source,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// Resolution is the per-selector grounding result.
type Resolution struct {
	Selector   Selector    `json:"selector,omitempty"`
	Status     Status      `json:"status"`
	Reason     string      `json:"reason,omitempty"`
	Assets     []Asset     `json:"assets,omitempty"`
	Candidates []Candidate `json:"candidates,omitempty"`
}

// Result contains all per-selector resolutions and flattened resolved assets.
type Result struct {
	Resolutions []Resolution `json:"resolutions"`
	Assets      []Asset      `json:"assets,omitempty"`
}

// Resolver resolves file selectors against page and conversation context.
type Resolver struct{}

// NewResolver creates a resolver.
func NewResolver() Resolver {
	return Resolver{}
}

// Resolve grounds selectors and returns per-selector plus flattened assets.
func Resolve(req Request) Result {
	return NewResolver().Resolve(req)
}
