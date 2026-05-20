package entities

import (
	"regexp"
	"strings"
)

var VariablePattern = regexp.MustCompile(`\{\{#([a-zA-Z0-9_]{1,50}(?:\.[a-zA-Z_][a-zA-Z0-9_]{0,29}){1,10})#\}\}`)

func (vp *VariablePool) ConvertTemplate(template string) *SegmentGroup {
	parts := VariablePattern.Split(template, -1)
	matches := VariablePattern.FindAllString(template, -1)

	segments := make([]Segment, 0, 10)

	for i, part := range parts {
		if part != "" {
			segments = append(segments, vp.buildSegment(part))
		}

		if i < len(matches) {
			match := matches[i]
			varName := strings.Trim(match, "{#}")

			if strings.Contains(varName, ".") {
				// Use GetWithPath for nested variables
				if variable := vp.GetWithPath(strings.Split(varName, ".")); variable != nil {
					segments = append(segments, variable)
				}
			} else {
				// Fallback to simple Get for non-nested
				if variable := vp.Get([]string{varName}); variable != nil {
					segments = append(segments, variable)
				}
			}
		}
	}

	return &SegmentGroup{Value: segments}
}
