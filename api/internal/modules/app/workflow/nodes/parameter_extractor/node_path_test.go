package parameterextractor

import (
	"context"
	"errors"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
)

func TestExecuteRun_UsesNestedQuerySelector(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"start", "payload"}, map[string]any{
		"text": "提取名字",
	})

	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID:            "param-node",
			UserID:            "user-1",
			APPID:             "app-1",
			GraphRuntimeState: entities.NewGraphRuntimeState(vp),
		},
		nodeData: NodeData{
			Model: ModelConfig{
				Provider:         "openai",
				Name:             "gpt-4o-mini",
				Mode:             "chat",
				CompletionParams: map[string]any{},
			},
			Query: []string{"start", "payload", "text"},
			Parameters: []ParameterConfig{
				{
					Name:     "name",
					Type:     ParameterTypeString,
					Required: true,
				},
			},
		},
		llmInvoker: &mockLLMInvoker{
			result: &InvokeResult{
				Text:   `{"name":"alice"}`,
				Finish: "stop",
			},
		},
	}

	result, err := node.executeRun(context.Background(), make(chan *shared.NodeEventCh, 1))
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}
	if result.Status != shared.SUCCEEDED {
		t.Fatalf("result.Status = %s, want %s", result.Status, shared.SUCCEEDED)
	}
	if got := result.Inputs["query"]; got != "提取名字" {
		t.Fatalf("inputs[query] = %#v, want %q", got, "提取名字")
	}
	if got := result.Outputs["name"]; got != "alice" {
		t.Fatalf("outputs[name] = %#v, want %q", got, "alice")
	}
	if got := node.llmInvoker.(*mockLLMInvoker).lastRequest.ModelSlug; got != "gpt-4o-mini" {
		t.Fatalf("invoked model slug = %q, want %q", got, "gpt-4o-mini")
	}
}

func TestFetchFiles_UsesNestedSelector(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"start", "payload"}, map[string]any{
		"attachment": map[string]any{
			"type":            "image",
			"transfer_method": "remote_url",
			"upload_file_id":  "file-1",
			"id":              "file-1",
			"workspace_id":    "ws-1",
			"url":             "https://example.com/paper.jpg",
			"filename":        "paper.jpg",
			"extension":       "jpg",
			"mime_type":       "image/jpeg",
		},
	})

	node := &Node{}
	files, err := node.fetchFiles(vp, []string{"start", "payload", "attachment"})
	if err != nil {
		t.Fatalf("fetchFiles() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(files))
	}
	if files[0] == nil {
		t.Fatalf("expected non-nil file")
	}
}

func TestExecuteRun_BillingFailureReturnsError(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"start", "query"}, "提取名字")

	billingErr := errors.Join(
		errors.New("all providers failed"),
		&gateway.BillingUserError{
			Kind:  gateway.BillingUserErrorKindPrivateChannelBalanceInsufficient,
			Cause: gateway.ErrInsufficientBalance,
		},
	)

	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID:            "param-node",
			UserID:            "user-1",
			APPID:             "app-1",
			GraphRuntimeState: entities.NewGraphRuntimeState(vp),
		},
		nodeData: NodeData{
			Model: ModelConfig{
				Provider:         "openai",
				Name:             "gpt-4o-mini",
				Mode:             "chat",
				CompletionParams: map[string]any{},
			},
			Query: []string{"start", "query"},
			Parameters: []ParameterConfig{
				{
					Name:     "name",
					Type:     ParameterTypeString,
					Required: true,
				},
			},
		},
		llmInvoker: &mockLLMInvoker{
			failCount: 1,
			err:       billingErr,
		},
	}

	result, err := node.executeRun(context.Background(), make(chan *shared.NodeEventCh, 1))
	if err == nil {
		t.Fatal("expected billing error, got nil")
	}
	if result == nil {
		t.Fatal("expected failure result, got nil")
	}
	if result.Status != shared.FAILED {
		t.Fatalf("result.Status = %s, want %s", result.Status, shared.FAILED)
	}

	var userErr *gateway.BillingUserError
	if !errors.As(err, &userErr) {
		t.Fatalf("errors.As(err, *BillingUserError) = false, err = %v", err)
	}
	if !errors.As(result.Err, &userErr) {
		t.Fatalf("errors.As(result.Err, *BillingUserError) = false, err = %v", result.Err)
	}
	if userErr.Kind != gateway.BillingUserErrorKindPrivateChannelBalanceInsufficient {
		t.Fatalf("userErr.Kind = %q, want %q", userErr.Kind, gateway.BillingUserErrorKindPrivateChannelBalanceInsufficient)
	}
}
