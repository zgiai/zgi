package service

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/contracts"
	datasetindexing "github.com/zgiai/zgi/api/internal/modules/dataset/indexing"
)

func TestParseArtifactChunkTransformServiceTransformsWithoutSideEffects(t *testing.T) {
	svc := NewParseArtifactChunkTransformService(nil, nil, nil, nil)
	chunks, err := svc.Transform(context.Background(), ParseArtifactChunkTransformInput{
		TenantID:  "org-1",
		IndexType: datasetindexing.ParagraphIndex,
		Artifact: &contracts.ParseArtifact{
			SourceType: contracts.ParseSourceTypeUploadFile,
			Elements: []contracts.ParsedElement{
				{Type: "text", Content: "hello", Ordinal: 1},
				{Type: "text", Content: "world", Ordinal: 2},
			},
		},
		ProcessOptions: &datasetindexing.ProcessOptions{Mode: "automatic"},
	})
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected transformed chunks")
	}
}

func TestParseArtifactChunkTransformServiceUsesParentChildSlidingWindowFallback(t *testing.T) {
	svc := NewParseArtifactChunkTransformService(nil, nil, nil, nil)
	runes := make([]rune, 3200)
	for i := range runes {
		runes[i] = rune(0x4e00 + i)
	}
	result, err := svc.TransformAuto(context.Background(), ParseArtifactAutoChunkTransformInput{
		TenantID: "org-1",
		FileName: "article.md",
		Artifact: &contracts.ParseArtifact{
			FileName: "article.md",
			Elements: []contracts.ParsedElement{{Type: "text", Content: string(runes), Ordinal: 1}},
		},
	})
	if err != nil {
		t.Fatalf("TransformAuto: %v", err)
	}
	if result.IndexType != datasetindexing.ParentChildIndex {
		t.Fatalf("index type = %q, want parent-child", result.IndexType)
	}
	if result.ProcessOptions.Mode != "hierarchical" || result.ProcessOptions.ProcessRule["parent_mode"] != "paragraph" {
		t.Fatalf("process options = %#v", result.ProcessOptions)
	}
	if result.Routing["matched"] != false {
		t.Fatalf("routing = %#v, want fallback route", result.Routing)
	}
	if len(result.Chunks) < 2 {
		t.Fatalf("got %d parent chunks, want multiple sliding windows", len(result.Chunks))
	}

	for parentIndex, parent := range result.Chunks {
		parentRunes := []rune(parent.Content)
		if len(parentRunes) > datasetindexing.DefaultParagraphParentMaxChars {
			t.Fatalf("parent %d size = %d, want <= %d", parentIndex, len(parentRunes), datasetindexing.DefaultParagraphParentMaxChars)
		}
		if parentIndex > 0 {
			previousRunes := []rune(result.Chunks[parentIndex-1].Content)
			if string(previousRunes[len(previousRunes)-datasetindexing.DefaultParagraphParentOverlapChars:]) != string(parentRunes[:datasetindexing.DefaultParagraphParentOverlapChars]) {
				t.Fatalf("parents %d/%d do not overlap by %d characters", parentIndex-1, parentIndex, datasetindexing.DefaultParagraphParentOverlapChars)
			}
		}
		if len(parent.Children) == 0 {
			t.Fatalf("parent %d has no child chunks", parentIndex)
		}
		for childIndex, child := range parent.Children {
			childRunes := []rune(child.Content)
			if len(childRunes) > datasetindexing.DefaultParagraphChildMaxChars {
				t.Fatalf("parent %d child %d size = %d, want <= %d", parentIndex, childIndex, len(childRunes), datasetindexing.DefaultParagraphChildMaxChars)
			}
			if childIndex > 0 {
				previousChildRunes := []rune(parent.Children[childIndex-1].Content)
				if string(previousChildRunes[len(previousChildRunes)-datasetindexing.DefaultParagraphChildOverlapChars:]) != string(childRunes[:datasetindexing.DefaultParagraphChildOverlapChars]) {
					t.Fatalf("parent %d children %d/%d do not overlap by %d characters", parentIndex, childIndex-1, childIndex, datasetindexing.DefaultParagraphChildOverlapChars)
				}
			}
		}
	}
}

func TestParseArtifactChunkTransformServiceRoutesVisionImagesToFullDoc(t *testing.T) {
	svc := NewParseArtifactChunkTransformService(nil, nil, nil, nil)
	markdown := "# 医院楼层导览\n\n## 8F\n\n- 儿科一病区\n\n## 7F\n\n- 急诊医学科病区"
	result, err := svc.TransformAuto(context.Background(), ParseArtifactAutoChunkTransformInput{
		TenantID: "org-1",
		FileName: "directory.jpg",
		Artifact: &contracts.ParseArtifact{
			FileName:   "directory.jpg",
			EngineUsed: contracts.ParseEngineVLM,
			Markdown:   markdown,
			Elements: []contracts.ParsedElement{
				{Type: "text", Content: markdown, Ordinal: 1},
			},
		},
	})
	if err != nil {
		t.Fatalf("TransformAuto: %v", err)
	}
	if result.IndexType != datasetindexing.ParentChildIndex {
		t.Fatalf("index type = %q, want parent-child", result.IndexType)
	}
	if result.Routing["route_name"] != "vision_image_full_doc" || result.Routing["matched"] != true {
		t.Fatalf("routing = %#v", result.Routing)
	}
	if result.ProcessOptions.Mode != "hierarchical" || result.ProcessOptions.ProcessRule["parent_mode"] != "full-doc" {
		t.Fatalf("process options = %#v", result.ProcessOptions)
	}
	if len(result.Chunks) != 1 || result.Chunks[0].Content != markdown {
		t.Fatalf("full-doc chunks = %#v", result.Chunks)
	}
	if len(result.Chunks[0].Children) == 0 {
		t.Fatalf("expected full-doc child chunks, got %#v", result.Chunks[0])
	}
}
