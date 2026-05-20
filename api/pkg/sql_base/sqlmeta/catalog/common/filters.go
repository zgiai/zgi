package common

// FilterCondition represents a SQL snippet and its args. Concrete repositories can compose them.
type FilterCondition struct {
	Clause string
	Args   []any
}

// Merge combines conditions into a single slice. Helper exists so callers stay concise.
func Merge(parts ...FilterCondition) []FilterCondition {
	out := make([]FilterCondition, 0, len(parts))
	for _, part := range parts {
		if part.Clause == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}
