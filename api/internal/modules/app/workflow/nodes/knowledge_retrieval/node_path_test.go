package knowledgeretrieval

import (
	"context"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

func TestNode_ExecuteRun_UsesNestedQuerySelector(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"source", "payload"}, map[string]any{
		"value": "",
	})

	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID:            "kr-node-1",
			GraphRuntimeState: entities.NewGraphRuntimeState(vp),
		},
		NodeData: NodeData{
			QueryVariableSelector: []string{"source", "payload", "value"},
		},
	}

	result, err := node.executeRun(context.Background(), nil)
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}

	if result.Status != shared.FAILED {
		t.Fatalf("status = %v, want FAILED", result.Status)
	}
	if result.ErrMsg != "Query is required." {
		t.Fatalf("err msg = %q, want %q", result.ErrMsg, "Query is required.")
	}
}

func TestNode_ConvertHitsToRetrieverResourcesNormalizesKnowledgeImageURLs(t *testing.T) {
	prevConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		App: config.AppConfig{
			FilesURL: "https://api.lingyoungai.com",
		},
	}
	t.Cleanup(func() {
		config.GlobalConfig = prevConfig
	})

	node := &Node{}
	docs := []DocumentHit{{
		Provider:    "zgi",
		Score:       0.91,
		PageContent: "黄芪图片：![figure](images/huangqi.png)",
		Metadata: map[string]any{
			"dataset_id":       "dataset-1",
			"dataset_name":     "Herbs",
			"document_id":      "document-1",
			"document_name":    "黄芪",
			"data_source_type": "upload",
			"segment_id":       "segment-1",
		},
	}}

	resources, contextText, err := node.convertHitsToRetrieverResources(docs)
	if err != nil {
		t.Fatalf("convertHitsToRetrieverResources() error = %v", err)
	}
	if len(resources) != 1 || resources[0].Content == nil {
		t.Fatalf("resources = %#v, want one resource with content", resources)
	}

	want := "https://api.lingyoungai.com/images/huangqi.png"
	if !strings.Contains(*resources[0].Content, want) {
		t.Fatalf("resource content missing normalized URL: %q", *resources[0].Content)
	}
	if !strings.Contains(contextText, want) {
		t.Fatalf("context missing normalized URL: %q", contextText)
	}
}
