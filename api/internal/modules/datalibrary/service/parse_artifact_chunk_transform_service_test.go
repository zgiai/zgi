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
