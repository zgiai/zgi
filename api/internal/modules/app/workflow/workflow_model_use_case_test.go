package workflow_test

import (
	"testing"

	knowledgeretrieval "github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/knowledge_retrieval"
	llmnode "github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/llm"
	parameterextractor "github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/parameter_extractor"
	sqlgenerator "github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/sqlgenerator"
)

func TestWorkflowModelNodesUseWorkflowAppType(t *testing.T) {
	tests := []struct {
		name    string
		appType string
	}{
		{name: "llm", appType: llmnode.AppType},
		{name: "parameter extractor", appType: parameterextractor.AppType},
		{name: "sql generator", appType: sqlgenerator.AppType},
		{name: "knowledge retrieval", appType: knowledgeretrieval.AppType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.appType != "workflow" {
				t.Fatalf("AppType = %q, want %q", tt.appType, "workflow")
			}
		})
	}
}
