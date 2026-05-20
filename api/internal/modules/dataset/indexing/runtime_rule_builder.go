package indexing

// RuntimeRuleBuilder builds runtime process rules for routed documents.
type RuntimeRuleBuilder struct{}

// NewRuntimeRuleBuilder creates a runtime rule builder.
func NewRuntimeRuleBuilder() *RuntimeRuleBuilder {
	return &RuntimeRuleBuilder{}
}

// BuildTableRule returns the minimal phase-1 process rule for table routing.
func (b *RuntimeRuleBuilder) BuildTableRule() (string, map[string]interface{}) {
	return "table", map[string]interface{}{}
}

// BuildSectionRule returns the mode and rules for section-based chunking.
func (b *RuntimeRuleBuilder) BuildSectionRule() (string, map[string]interface{}) {
	return "hierarchical", map[string]interface{}{
		"parent_mode": "section",
		"subchunk_segmentation": map[string]interface{}{
			"separator":     "\n\n",
			"max_tokens":    500,
			"chunk_overlap": 50,
		},
	}
}

// BuildFullDocRule returns the mode and rules for full-document chunking.
func (b *RuntimeRuleBuilder) BuildFullDocRule() (string, map[string]interface{}) {
	return "hierarchical", map[string]interface{}{
		"parent_mode": "full-doc",
		"subchunk_segmentation": map[string]interface{}{
			"separator":     "\n\n",
			"max_tokens":    500,
			"chunk_overlap": 50,
		},
	}
}
