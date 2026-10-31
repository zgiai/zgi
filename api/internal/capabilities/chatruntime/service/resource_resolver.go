package service

import (
	"strings"
)

const (
	resourceResolverCandidateLimit = 8
	resourceTypeFile               = "file"
)

// ResourceResolutionStatus describes whether a planner resource reference could
// be grounded to concrete resources from the current chat/page context.
type ResourceResolutionStatus string

const (
	// ResourceResolutionStatusResolved means the resolver found a concrete file.
	ResourceResolutionStatusResolved ResourceResolutionStatus = "resolved"
	// ResourceResolutionStatusAmbiguous means more than one visible candidate matched.
	ResourceResolutionStatusAmbiguous ResourceResolutionStatus = "ambiguous"
	// ResourceResolutionStatusNotFound means no visible/context candidate matched.
	ResourceResolutionStatusNotFound ResourceResolutionStatus = "not_found"
)

// PlannerResourceRef is the structured reference emitted by the semantic planner.
//
// The shape is intentionally tolerant: newer planner workers can use typed fields,
// while earlier experiments can pass equivalent values through Metadata.
type PlannerResourceRef struct {
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

// ResourceResolverInput contains the bounded context used to ground planner
// references. OperationContext should be the raw page context when available;
// NormalizedOperationContext can carry the ledger summary as a fallback.
type ResourceResolverInput struct {
	OperationContext           map[string]interface{} `json:"operation_context,omitempty"`
	NormalizedOperationContext map[string]interface{} `json:"normalized_operation_context,omitempty"`
	AttachmentFiles            []ResourceCandidate    `json:"attachment_files,omitempty"`
}

// ResourceCandidate is a file-like resource visible to the current AIChat turn.
type ResourceCandidate struct {
	Type           string                 `json:"type,omitempty"`
	ID             string                 `json:"id,omitempty"`
	Name           string                 `json:"name,omitempty"`
	Title          string                 `json:"title,omitempty"`
	Source         string                 `json:"source,omitempty"`
	Extension      string                 `json:"extension,omitempty"`
	MimeType       string                 `json:"mime_type,omitempty"`
	FileType       string                 `json:"file_type,omitempty"`
	Selected       bool                   `json:"selected,omitempty"`
	Recent         bool                   `json:"recent,omitempty"`
	Visible        bool                   `json:"visible,omitempty"`
	VisibleOrdinal int                    `json:"visible_ordinal,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// ResourceRef is a grounded asset reference used by chat runtime planning and
// tool governance context.
type ResourceRef struct {
	Type     string                 `json:"type,omitempty"`
	ID       string                 `json:"id,omitempty"`
	Name     string                 `json:"name,omitempty"`
	Source   string                 `json:"source,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ResourceResolution is the per-reference grounding result returned to the
// chat runtime coordinator.
type ResourceResolution struct {
	Ref        PlannerResourceRef       `json:"ref,omitempty"`
	Status     ResourceResolutionStatus `json:"status"`
	Reason     string                   `json:"reason,omitempty"`
	Resources  []ResourceRef            `json:"resources,omitempty"`
	FileIDs    []string                 `json:"file_ids,omitempty"`
	Candidates []ResourceCandidate      `json:"candidates,omitempty"`
}

// ResourceResolverResult contains all per-reference results and the flattened
// resolved action resources/file IDs.
type ResourceResolverResult struct {
	Results   []ResourceResolution `json:"results"`
	Resources []ResourceRef        `json:"resources,omitempty"`
	FileIDs   []string             `json:"file_ids,omitempty"`
}

// ResourceResolver grounds planner resource references against chat/page context.
type ResourceResolver struct{}

func fileTypeExtensions(fileType string) []string {
	switch normalizeResourceToken(fileType) {
	case "pdf":
		return []string{"pdf"}
	case "excel", "spreadsheet":
		return []string{"xls", "xlsx", "xlsm", "xlsb"}
	case "csv":
		return []string{"csv"}
	case "image":
		return []string{"png", "jpg", "jpeg", "gif", "webp", "bmp", "svg"}
	case "document":
		return []string{"pdf", "doc", "docx", "txt", "md", "rtf"}
	default:
		extension := normalizedResolverFileExtension(fileType)
		if extension == "" {
			return nil
		}
		return []string{extension}
	}
}

func fileNameExtension(value string) string {
	value = strings.TrimSpace(value)
	dot := strings.LastIndex(value, ".")
	if dot < 0 || dot == len(value)-1 {
		return ""
	}
	return normalizedResolverFileExtension(value[dot+1:])
}

func normalizedResolverFileExtension(value string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), ".")
}

func normalizeResourceToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
