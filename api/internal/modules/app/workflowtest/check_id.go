package workflowtest

import (
	"fmt"
	"strings"
)

func uniqueWorkflowTestCheckID(seen map[string]int, candidate string, fallback string) string {
	base := strings.TrimSpace(candidate)
	if base == "" {
		base = strings.TrimSpace(fallback)
	}
	if base == "" {
		base = "check"
	}
	if seen[base] == 0 {
		seen[base] = 1
		return base
	}
	for next := seen[base] + 1; ; next++ {
		value := fmt.Sprintf("%s_%d", base, next)
		if seen[value] == 0 {
			seen[base] = next
			seen[value] = 1
			return value
		}
	}
}
