package documentextractor

import (
	"context"
	"testing"

	workflowfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

type stubContentExtractor struct{}

func (s *stubContentExtractor) ExtractFileContent(ctx context.Context, fileID string, tenantID string) (*workflowfile.FileContent, error) {
	return &workflowfile.FileContent{
		FileID:  fileID,
		Content: "nested text",
	}, nil
}

func (s *stubContentExtractor) ExtractMultipleFiles(ctx context.Context, fileIDs []string, tenantID string) ([]*workflowfile.FileContent, error) {
	results := make([]*workflowfile.FileContent, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		results = append(results, &workflowfile.FileContent{
			FileID:  fileID,
			Content: "nested text",
		})
	}
	return results, nil
}

func (s *stubContentExtractor) ProcessFileVariable(ctx context.Context, variableName string, fileData map[string]interface{}, tenantID string) (map[string]interface{}, error) {
	return nil, nil
}

func (s *stubContentExtractor) ProcessFileListVariable(ctx context.Context, variableName string, fileList []interface{}, tenantID string) (map[string]interface{}, error) {
	return nil, nil
}

func TestExecuteRun_UsesNestedFileSelector(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"start", "payload"}, map[string]any{
		"attachment": map[string]any{
			"type":            "document",
			"transfer_method": "local_file",
			"upload_file_id":  "file-1",
			"id":              "file-1",
			"workspace_id":    "ws-1",
			"filename":        "paper.pdf",
			"extension":       "pdf",
			"mime_type":       "application/pdf",
		},
	})

	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID:            "doc-node",
			TenantID:          "tenant-1",
			GraphRuntimeState: entities.NewGraphRuntimeState(vp),
			WorkflowType:      "workflow",
			Graph:             &entities.Graph{},
			PreviousNodeID:    nil,
			WorkflowCallDepth: 0,
			GraphConfig:       nil,
			InvokeFrom:        "",
			UserFrom:          "",
			UserID:            "",
			APPID:             "",
			WorkflowID:        "",
			InstanceID:        "",
		},
		NodeData: NodeData{
			VariableSelector: []string{"start", "payload", "attachment"},
		},
		contentExtractor: &stubContentExtractor{},
	}

	result, err := node.executeRun(context.Background(), make(chan *shared.NodeEventCh, 1))
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}
	if result.Status != shared.SUCCEEDED {
		t.Fatalf("result.Status = %s, want %s", result.Status, shared.SUCCEEDED)
	}

	texts, ok := result.Outputs["text"].([]string)
	if !ok {
		t.Fatalf("outputs[text] type = %T, want []string", result.Outputs["text"])
	}
	if len(texts) != 1 || texts[0] != "nested text" {
		t.Fatalf("outputs[text] = %#v, want []string{\"nested text\"}", texts)
	}
}
