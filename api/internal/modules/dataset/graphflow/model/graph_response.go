package model

// GraphDataResponse represents the complete graph data for frontend visualization
type GraphDataResponse struct {
	Nodes      []GraphNode     `json:"nodes"`
	Edges      []GraphEdge     `json:"edges"`
	Categories []GraphCategory `json:"categories"`
}

// GraphNode represents an entity node in the knowledge graph
type GraphNode struct {
	ID       string        `json:"id"`       // Prefixed entity ID (e.g., "ent:uuid")
	Label    string        `json:"label"`    // Display name
	Category string        `json:"category"` // Type key (for category matching)
	Data     GraphNodeData `json:"data"`     // Additional node metadata
}

// GraphNodeData contains detailed node information
type GraphNodeData struct {
	Description string             `json:"description"`
	Sources     []GraphNodeSource  `json:"sources"` // Document sources with weights
}

// GraphNodeSource represents a document source for an entity
type GraphNodeSource struct {
	Doc    GraphSourceDoc `json:"doc"`
	Weight int            `json:"weight"` // Mention count or calculated weight
}

// GraphSourceDoc represents a source document reference
type GraphSourceDoc struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// GraphEdge represents a relationship edge in the knowledge graph
type GraphEdge struct {
	Source string `json:"source"` // Source node ID (must match GraphNode.ID)
	Target string `json:"target"` // Target node ID (must match GraphNode.ID)
	Label  string `json:"label"`  // Relationship type / predicate
}

// GraphCategory represents an entity type category for legend/filtering
type GraphCategory struct {
	ID    string           `json:"id"`    // Type key (e.g., "Person")
	Label GraphCategoryLabel `json:"label"` // Multi-language labels
}

// GraphCategoryLabel contains localized category labels
type GraphCategoryLabel struct {
	ZhHans string `json:"zh-Hans"`
	EnUS   string `json:"en-US"`
}
