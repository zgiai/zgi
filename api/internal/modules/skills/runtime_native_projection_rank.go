package skills

import (
	"sort"
	"strings"
)

// RankNativeToolProjections orders already-authorized projections by relevance
// without selecting or dropping tools. Continuation priority is retained ahead
// of query matches; the common projection pipeline still owns limits and budget.
func RankNativeToolProjections(projections []NativeToolProjection, query string) []NativeToolProjection {
	if projections == nil {
		return nil
	}
	query = strings.TrimSpace(query)
	runeCount := 0
	for index := range query {
		if runeCount == 512 {
			query = query[:index]
			break
		}
		runeCount++
	}
	query = nativeProjectionSearchText(query)

	type rankedProjection struct {
		projection NativeToolProjection
		priority   int
		score      int
	}
	ranked := make([]rankedProjection, 0, len(projections))
	for _, projection := range projections {
		priority := projection.Binding.ProjectionPriority
		if projection.Binding.Pinned && priority < 2 {
			priority = 2
		}
		if priority < 0 {
			priority = 0
		}
		if priority > 2 {
			priority = 2
		}
		score := nativeSearchFieldScore([]string{
			nativeProjectionSearchText(projection.Name),
			nativeProjectionSearchText(projection.NameScope),
			nativeProjectionSearchText(projection.Description),
		}, query)
		projection.Binding = cloneNativeToolBinding(projection.Binding)
		if score > 0 && projection.Binding.ProjectionPriority < 1 {
			projection.Binding.ProjectionPriority = 1
		}
		ranked = append(ranked, rankedProjection{projection: projection, priority: priority, score: score})
	}
	sort.SliceStable(ranked, func(left, right int) bool {
		if ranked[left].priority != ranked[right].priority {
			return ranked[left].priority > ranked[right].priority
		}
		return ranked[left].score > ranked[right].score
	})
	out := make([]NativeToolProjection, len(ranked))
	for index := range ranked {
		out[index] = ranked[index].projection
	}
	return out
}

func nativeProjectionSearchText(value string) string {
	value = strings.Map(func(current rune) rune {
		switch current {
		case '_', '/', '-', '.':
			return ' '
		default:
			return current
		}
	}, value)
	return strings.Join(strings.Fields(value), " ")
}
