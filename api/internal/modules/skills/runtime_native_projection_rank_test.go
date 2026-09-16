package skills

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestRankNativeToolProjectionsRanksAllCandidatesWithoutSelecting(t *testing.T) {
	projections := make([]NativeToolProjection, 0, 130)
	for index := 0; index < 129; index++ {
		projections = append(projections, NativeToolProjection{Name: fmt.Sprintf("unrelated_%03d", index)})
	}
	projections = append(projections, NativeToolProjection{
		Name: "search_web", NameScope: "connector/web-search", Description: "Find public webpages",
		Binding: NativeToolBinding{SkillID: "web-search", ToolName: "search_web"},
	})

	ranked := RankNativeToolProjections(projections, "search web")
	if len(ranked) != len(projections) {
		t.Fatalf("ranking selected %d of %d tools; selection belongs to the projection pipeline", len(ranked), len(projections))
	}
	if ranked[0].Name != "search_web" || ranked[0].Binding.ProjectionPriority != 1 {
		t.Fatalf("relevant operation was not promoted: %#v", ranked[0])
	}
	for index, projection := range ranked[1:] {
		if projection.Name != projections[index].Name {
			t.Fatalf("equal-score tools lost stable order at %d: %q", index, projection.Name)
		}
	}
}

func TestRankNativeToolProjectionsSearchesPublicFieldsAndSeparators(t *testing.T) {
	for _, separator := range []string{"_", "/", "-", "."} {
		for _, field := range []string{"name", "scope", "description"} {
			t.Run(field+separator, func(t *testing.T) {
				projection := NativeToolProjection{Name: "matched"}
				value := "search" + separator + "web"
				switch field {
				case "name":
					projection.Name = value
				case "scope":
					projection.NameScope = value
				case "description":
					projection.Description = value
				}
				projections := []NativeToolProjection{
					{Name: "unrelated", Binding: NativeToolBinding{ToolName: "search web", ConnectorOperation: &NativeConnectorOperation{ConnectorID: "search", OperationID: "web"}}},
					{Name: "partial", Description: "search"},
					projection,
				}
				ranked := RankNativeToolProjections(projections, "SEARCH WEB")
				if ranked[0].Name != projection.Name || ranked[1].Name != "partial" || ranked[2].Name != "unrelated" {
					t.Fatalf("public field relevance mismatch: %v", nativeRankedProjectionNames(ranked))
				}
				if ranked[2].Binding.ProjectionPriority != 0 {
					t.Fatal("execution-only binding fields must not influence relevance")
				}
			})
		}
	}
}

func TestRankNativeToolProjectionsPreservesContinuationPriority(t *testing.T) {
	projections := []NativeToolProjection{
		{Name: "unrelated"},
		{Name: "search_web"},
		{Name: "continuation", Binding: NativeToolBinding{ProjectionPriority: 2}},
		{Name: "pinned", Binding: NativeToolBinding{Pinned: true}},
		{Name: "explicit_priority", Binding: NativeToolBinding{ProjectionPriority: 1}},
	}
	ranked := RankNativeToolProjections(projections, "search web")
	want := []string{"continuation", "pinned", "explicit_priority", "search_web", "unrelated"}
	if got := nativeRankedProjectionNames(ranked); !reflect.DeepEqual(got, want) {
		t.Fatalf("continuation order = %v, want %v", got, want)
	}
	if ranked[0].Binding.ProjectionPriority != 2 || !ranked[1].Binding.Pinned {
		t.Fatal("explicit priority flags were changed")
	}
	// The existing selection pipeline must retain the same priority order.
	selected := priorityOrderedNativeToolProjections(ranked)
	if got := nativeRankedProjectionNames(selected); !reflect.DeepEqual(got, want) {
		t.Fatalf("common projection priority reordered ranked tools: %v", got)
	}
}

func TestRankNativeToolProjectionsPreservesOriginalAndAliases(t *testing.T) {
	projection := NativeToolProjection{
		Name: "search_web", NameScope: "connector/web-search", Description: "search web",
		InputSchema: map[string]interface{}{"type": "object"},
		Binding: NativeToolBinding{
			SkillID: "native-web", ToolName: "search_web",
			ConnectorOperation: &NativeConnectorOperation{ConnectorID: "web-search", OperationID: "web.search"},
		},
	}
	projections := []NativeToolProjection{{Name: "unrelated"}, projection}
	wantOriginal := append([]NativeToolProjection(nil), projections...)
	operation := *projection.Binding.ConnectorOperation
	wantOriginal[1].Binding.ConnectorOperation = &operation
	ranked := RankNativeToolProjections(projections, "search web")
	if !reflect.DeepEqual(projections, wantOriginal) {
		t.Fatal("ranking mutated original projections")
	}
	if ranked[0].Name != projection.Name || ranked[0].NameScope != projection.NameScope ||
		ranked[0].Binding.SkillID != projection.Binding.SkillID || ranked[0].Binding.ToolName != projection.Binding.ToolName ||
		!reflect.DeepEqual(ranked[0].InputSchema, projection.InputSchema) {
		t.Fatal("ranking changed the public alias, execution identity, or schema")
	}
	ranked[0].Binding.ConnectorOperation.OperationID = "changed"
	ranked[0].Name = "changed"
	if !reflect.DeepEqual(projections, wantOriginal) {
		t.Fatal("ranked projection binding shares mutable state with original")
	}
}

func TestRankNativeToolProjectionsUsesUnicodeTokens(t *testing.T) {
	projections := []NativeToolProjection{{Name: "unrelated"}, {Name: "web", Description: "公开网页搜索"}}
	ranked := RankNativeToolProjections(projections, "请搜索公开网页")
	if ranked[0].Name != "web" || ranked[0].Binding.ProjectionPriority != 1 {
		t.Fatalf("Unicode token relevance not applied: %#v", ranked)
	}
}

func TestRankNativeToolProjectionsBoundsQueryByRunes(t *testing.T) {
	projections := []NativeToolProjection{{Name: "unrelated"}, {Name: "search_web"}, {Name: "unicode", Description: "公开"}}
	for _, query := range []string{
		strings.Repeat("x", 512) + " search web",
		strings.Repeat("界", 512) + " search web",
	} {
		ranked := RankNativeToolProjections(projections, query)
		if !reflect.DeepEqual(nativeRankedProjectionNames(ranked), nativeRankedProjectionNames(projections)) {
			t.Fatal("query suffix beyond 512 runes influenced ranking")
		}
	}
	ranked := RankNativeToolProjections(projections, strings.Repeat("界", 510)+"公开 search web")
	if ranked[0].Name != "unicode" || ranked[0].Binding.ProjectionPriority != 1 {
		t.Fatal("multibyte query was clipped by bytes instead of runes")
	}
}

func TestRankNativeToolProjectionsEmptyQueryKeepsStableTies(t *testing.T) {
	projections := []NativeToolProjection{{Name: "z"}, {Name: "a"}, {Name: "m"}}
	ranked := RankNativeToolProjections(projections, " \t\n")
	if !reflect.DeepEqual(nativeRankedProjectionNames(ranked), nativeRankedProjectionNames(projections)) {
		t.Fatal("empty-query ties did not retain original order")
	}
	for _, projection := range ranked {
		if projection.Binding.ProjectionPriority != 0 {
			t.Fatal("empty query promoted a tool")
		}
	}
	if RankNativeToolProjections(nil, "query") != nil {
		t.Fatal("nil input must stay nil")
	}
}

func nativeRankedProjectionNames(projections []NativeToolProjection) []string {
	out := make([]string, len(projections))
	for index := range projections {
		out[index] = projections[index].Name
	}
	return out
}
