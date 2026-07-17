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

// BuildElementGroupRule returns the standard element-order parent-child rule.
func (b *RuntimeRuleBuilder) BuildElementGroupRule() (string, map[string]interface{}) {
	return "hierarchical", map[string]interface{}{
		"parent_mode":           "element_group",
		"parent_min_chars":      1000,
		"parent_target_chars":   1200,
		"parent_max_chars":      1500,
		"child_min_chars":       120,
		"child_target_chars":    220,
		"child_max_chars":       256,
		"child_overlap_chars":   30,
		"table_child_max_chars": 256,
		"subchunk_segmentation": map[string]interface{}{
			"separator":     "\n",
			"max_tokens":    220,
			"chunk_overlap": 30,
		},
	}
}

// BuildSectionRule returns the mode and rules for section-based chunking.
func (b *RuntimeRuleBuilder) BuildSectionRule() (string, map[string]interface{}) {
	return b.BuildElementGroupRule()
}

// BuildFullDocRule returns the mode and rules for full-document chunking.
func (b *RuntimeRuleBuilder) BuildFullDocRule() (string, map[string]interface{}) {
	return b.BuildElementGroupRule()
}
