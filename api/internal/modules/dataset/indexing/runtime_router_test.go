package indexing

import (
	"context"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
)

func TestRuntimeRouterRoutesShortDomainDocumentToFullDoc(t *testing.T) {
	router := NewRuntimeRouter(context.Background(), nil, nil, "")

	decision, err := router.Route(RouterInput{
		DataSourceType: "upload_file",
		DocExt:         ".md",
		ExtractedOutput: &dto.ExtractOutput{
			Elements: []dto.ExtractElement{
				{Type: "text", Content: "resume\nshort profile"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Route returned error: %v", err)
	}
	if decision == nil || !decision.Matched {
		t.Fatalf("expected matched full-doc route, got %#v", decision)
	}
	if decision.RouteName != "full_doc_model" {
		t.Fatalf("route name = %q, want full_doc_model", decision.RouteName)
	}
	if decision.TargetRules["parent_mode"] != "full-doc" {
		t.Fatalf("parent_mode = %v, want full-doc", decision.TargetRules["parent_mode"])
	}
}

func TestRuntimeRouterDoesNotRouteShortUnknownDocumentToFullDoc(t *testing.T) {
	router := NewRuntimeRouter(context.Background(), nil, nil, "")

	decision, err := router.Route(RouterInput{
		DataSourceType: "upload_file",
		DocExt:         ".md",
		ExtractedOutput: &dto.ExtractOutput{
			Elements: []dto.ExtractElement{
				{Type: "text", Content: "short document without a special domain"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Route returned error: %v", err)
	}
	if decision == nil {
		t.Fatal("expected decision")
	}
	if decision.RouteName == "full_doc_model" || decision.Matched {
		t.Fatalf("short document alone should not route to full-doc: %#v", decision)
	}
}

func TestRuntimeRouterDoesNotRouteLongDomainDocumentToFullDoc(t *testing.T) {
	router := NewRuntimeRouter(context.Background(), nil, nil, "")

	decision, err := router.Route(RouterInput{
		DataSourceType: "upload_file",
		DocExt:         ".md",
		ExtractedOutput: &dto.ExtractOutput{
			Elements: []dto.ExtractElement{
				{Type: "text", Content: "resume\n" + strings.Repeat("x", maxFullDocParentChunkRunes+1)},
			},
		},
	})
	if err != nil {
		t.Fatalf("Route returned error: %v", err)
	}
	if decision == nil {
		t.Fatal("expected decision")
	}
	if decision.RouteName == "full_doc_model" {
		t.Fatalf("long document should not route to full-doc: %#v", decision)
	}
	if decision.Matched {
		t.Fatalf("expected no fallback match for long non-section document, got %#v", decision)
	}
	if _, ok := decision.RouteMeta["doc_domain"]; ok {
		t.Fatalf("long document should skip domain analysis: %#v", decision.RouteMeta)
	}
	if got, ok := decision.RouteMeta["extracted_word_count"].(int); !ok || got != maxFullDocParentChunkRunes+1 {
		t.Fatalf("extracted_word_count = %v, want > %d", decision.RouteMeta["extracted_word_count"], maxFullDocParentChunkRunes)
	}
}

func TestRuntimeRouterDoesNotCountTextForTableRoute(t *testing.T) {
	router := NewRuntimeRouter(context.Background(), nil, nil, "")

	decision, err := router.Route(RouterInput{
		DataSourceType: "upload_file",
		DocExt:         ".csv",
		ExtractedOutput: &dto.ExtractOutput{
			Elements: []dto.ExtractElement{
				{Type: "text", Content: strings.Repeat("x", maxFullDocParentChunkRunes+1)},
			},
		},
	})
	if err != nil {
		t.Fatalf("Route returned error: %v", err)
	}
	if decision == nil || decision.RouteName != "table_model" {
		t.Fatalf("expected table route, got %#v", decision)
	}
	if _, ok := decision.RouteMeta["extracted_word_count"]; ok {
		t.Fatalf("table route should not compute extracted_word_count: %#v", decision.RouteMeta)
	}
}

func TestRuntimeRouterRoutesLongSectionDocumentWithoutDomainAnalysis(t *testing.T) {
	router := NewRuntimeRouter(context.Background(), nil, nil, "")

	decision, err := router.Route(RouterInput{
		DataSourceType: "upload_file",
		DocExt:         ".md",
		ExtractedOutput: &dto.ExtractOutput{
			Elements: []dto.ExtractElement{
				{Type: "heading", Content: "Resume"},
				{Type: "text", Content: strings.Repeat("x", maxFullDocParentChunkRunes+1)},
				{Type: "heading", Content: "Experience"},
				{Type: "text", Content: "content"},
				{Type: "heading", Content: "Education"},
				{Type: "text", Content: "content"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Route returned error: %v", err)
	}
	if decision == nil || decision.RouteName != "section_model" {
		t.Fatalf("expected section route, got %#v", decision)
	}
	if _, ok := decision.RouteMeta["doc_domain"]; ok {
		t.Fatalf("long section document should skip domain analysis: %#v", decision.RouteMeta)
	}
}
